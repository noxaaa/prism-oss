package forward

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

const (
	maxTLSClientHelloBytes    = 8192
	maxProxyProtocolV1LineLen = 107
	udpSessionIdleTimeout     = 30 * time.Second
)

type Supervisor struct {
	mu        sync.Mutex
	listeners map[listenerKey]*listenerRuntime
	metrics   *metricsCounter
}

type metricsCounter struct {
	mu                  sync.Mutex
	tcpConnections      int64
	tcpConnectionEvents int64
	udpPackets          int64
	uploadBytes         int64
	downloadBytes       int64
	lastSnapshot        Metrics
	targets             map[targetMetricKey]*targetMetricCounter
	lastTargets         map[targetMetricKey]targetMetricSnapshot
	pendingDeltas       []agent.RuleTrafficDelta
	activeTargets       map[targetMetricKey]bool
	activeApplied       bool
	lastSnapshotAt      time.Time
}

type listenerKey struct {
	listenIP string
	protocol domain.Protocol
	port     int
}

type listenerRuntime struct {
	cancel context.CancelFunc
	stop   func()
	rules  *listenerRules
}

type stoppedListenerRuntime struct {
	key     listenerKey
	rules   []agent.RuleConfig
	runtime *listenerRuntime
}

type listenerRules struct {
	mu    sync.RWMutex
	rules []agent.RuleConfig
}

type udpSessionTable struct {
	mu       sync.Mutex
	listener net.PacketConn
	metrics  *metricsCounter
	sessions map[string]*udpSession
}

type udpSession struct {
	key           string
	ruleID        string
	targetID      string
	targetAddress string
	clientAddress net.Addr
	upstream      *net.UDPConn
	lastSeen      time.Time
}

func newListenerRules(rules []agent.RuleConfig) *listenerRules {
	store := &listenerRules{}
	store.set(rules)
	return store
}

func (store *listenerRules) set(rules []agent.RuleConfig) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.rules = append([]agent.RuleConfig(nil), rules...)
}

func (store *listenerRules) snapshot() []agent.RuleConfig {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return append([]agent.RuleConfig(nil), store.rules...)
}

func NewSupervisor() *Supervisor {
	return &Supervisor{
		listeners: make(map[listenerKey]*listenerRuntime),
		metrics:   newMetricsCounter(),
	}
}

func (supervisor *Supervisor) Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error {
	grouped, err := groupRulesByListener(snapshot.Rules)
	if err != nil {
		return err
	}
	if err := validateNoOverlappingListeners(grouped); err != nil {
		return err
	}
	next := make(map[listenerKey]*listenerRuntime, len(grouped))
	started := make(map[listenerKey]*listenerRuntime)
	var removed []*listenerRuntime
	supervisor.mu.Lock()
	old := supervisor.listeners
	preStopped := supervisor.stopOverlappingAbsentListenersLocked(grouped)
	for key, rules := range grouped {
		if runtime, ok := old[key]; ok {
			next[key] = runtime
			continue
		}
		runtime, err := supervisor.startListener(ctx, key, rules)
		if err != nil {
			applyErr := listenerBindApplyError(key, rules, err)
			for _, runtime := range started {
				runtime.stop()
			}
			if restoreErr := supervisor.restoreStoppedListenersLocked(ctx, preStopped); restoreErr != nil {
				applyErr.Message = fmt.Sprintf("%s; restore previous listeners: %v", applyErr.Error(), restoreErr)
			}
			supervisor.mu.Unlock()
			return applyErr
		}
		next[key] = runtime
		started[key] = runtime
	}
	for key, runtime := range old {
		if _, ok := next[key]; !ok {
			removed = append(removed, runtime)
		}
	}
	for key, rules := range grouped {
		next[key].rules.set(rules)
	}
	supervisor.listeners = next
	supervisor.mu.Unlock()

	for _, runtime := range removed {
		runtime.stop()
	}
	supervisor.metrics.setActiveTargets(snapshot.Rules)
	return nil
}

func (supervisor *Supervisor) stopOverlappingAbsentListenersLocked(grouped map[listenerKey][]agent.RuleConfig) []stoppedListenerRuntime {
	stopped := make([]stoppedListenerRuntime, 0)
	for oldKey, runtime := range supervisor.listeners {
		if _, stillPresent := grouped[oldKey]; stillPresent {
			continue
		}
		if !overlapsAnyListener(oldKey, grouped) {
			continue
		}
		delete(supervisor.listeners, oldKey)
		stopped = append(stopped, stoppedListenerRuntime{
			key:     oldKey,
			rules:   runtime.rules.snapshot(),
			runtime: runtime,
		})
		runtime.stop()
	}
	return stopped
}

func (supervisor *Supervisor) restoreStoppedListenersLocked(ctx context.Context, stopped []stoppedListenerRuntime) error {
	for _, stoppedRuntime := range stopped {
		if _, exists := supervisor.listeners[stoppedRuntime.key]; exists {
			continue
		}
		runtime, err := supervisor.startListener(ctx, stoppedRuntime.key, stoppedRuntime.rules)
		if err != nil {
			return err
		}
		supervisor.listeners[stoppedRuntime.key] = runtime
	}
	return nil
}

func overlapsAnyListener(key listenerKey, listeners map[listenerKey][]agent.RuleConfig) bool {
	for candidate := range listeners {
		if listenerKeysOverlap(key, candidate) {
			return true
		}
	}
	return false
}

func validateNoOverlappingListeners(listeners map[listenerKey][]agent.RuleConfig) error {
	keys := make([]listenerKey, 0, len(listeners))
	for key := range listeners {
		keys = append(keys, key)
	}
	for leftIndex := 0; leftIndex < len(keys); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(keys); rightIndex++ {
			if listenerKeysOverlap(keys[leftIndex], keys[rightIndex]) {
				return fmt.Errorf("overlapping listeners %s and %s", listenerKeyString(keys[leftIndex]), listenerKeyString(keys[rightIndex]))
			}
		}
	}
	return nil
}

func listenerKeyString(key listenerKey) string {
	return string(key.protocol) + "/" + key.listenIP + ":" + strconv.Itoa(key.port)
}

func listenerKeysOverlap(left listenerKey, right listenerKey) bool {
	return left.protocol == right.protocol &&
		left.port == right.port &&
		listenIPsOverlap(left.listenIP, right.listenIP)
}

func listenIPsOverlap(left string, right string) bool {
	return left == right || isWildcardListenIP(left) || isWildcardListenIP(right)
}

func isWildcardListenIP(value string) bool {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	if value == "" {
		return true
	}
	ip := net.ParseIP(value)
	return ip != nil && ip.IsUnspecified()
}

func (supervisor *Supervisor) Close() {
	supervisor.mu.Lock()
	listeners := supervisor.listeners
	supervisor.listeners = make(map[listenerKey]*listenerRuntime)
	supervisor.mu.Unlock()
	for _, runtime := range listeners {
		runtime.stop()
	}
}

func (supervisor *Supervisor) Metrics() Metrics {
	return supervisor.metrics.snapshot()
}

func (supervisor *Supervisor) AgentMetrics() agent.MetricsPayload {
	return supervisor.metrics.agentPayload(time.Now())
}

func (supervisor *Supervisor) startListener(ctx context.Context, key listenerKey, rules []agent.RuleConfig) (*listenerRuntime, error) {
	listenAddress := net.JoinHostPort(key.listenIP, strconv.Itoa(key.port))
	runtimeCtx, cancel := context.WithCancel(ctx)
	started := false
	defer func() {
		if !started {
			cancel()
		}
	}()
	var runtime *listenerRuntime
	var err error
	switch key.protocol {
	case domain.ProtocolTCP:
		runtime, err = supervisor.startTCPListener(runtimeCtx, cancel, listenAddress, rules)
	case domain.ProtocolUDP:
		runtime, err = supervisor.startUDPListener(runtimeCtx, cancel, listenAddress, rules)
	default:
		err = fmt.Errorf("unsupported listener protocol %s", key.protocol)
	}
	if err != nil {
		return nil, err
	}
	started = true
	return runtime, nil
}

func (supervisor *Supervisor) startTCPListener(ctx context.Context, cancel context.CancelFunc, listenAddress string, rules []agent.RuleConfig) (*listenerRuntime, error) {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, err
	}
	ruleStore := newListenerRules(rules)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go supervisor.proxyTCP(ctx, conn, ruleStore.snapshot())
		}
	}()
	return &listenerRuntime{
		cancel: cancel,
		rules:  ruleStore,
		stop: func() {
			cancel()
			_ = listener.Close()
			<-done
		},
	}, nil
}

func (supervisor *Supervisor) startUDPListener(ctx context.Context, cancel context.CancelFunc, listenAddress string, rules []agent.RuleConfig) (*listenerRuntime, error) {
	conn, err := net.ListenPacket("udp", listenAddress)
	if err != nil {
		return nil, err
	}
	ruleStore := newListenerRules(rules)
	sessions := newUDPSessionTable(conn, supervisor.metrics)
	done := make(chan struct{})
	go func() {
		defer close(done)
		cleanupTicker := time.NewTicker(udpSessionIdleTimeout)
		defer cleanupTicker.Stop()
		buffer := make([]byte, 65535)
		for {
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			n, clientAddress, err := conn.ReadFrom(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-ctx.Done():
						return
					case <-cleanupTicker.C:
						sessions.closeIdle(time.Now())
					default:
					}
					continue
				}
				return
			}
			payload := append([]byte(nil), buffer[:n]...)
			go supervisor.proxyUDP(ctx, sessions, clientAddress, ruleStore.snapshot(), payload)
			select {
			case <-cleanupTicker.C:
				sessions.closeIdle(time.Now())
			default:
			}
		}
	}()
	return &listenerRuntime{
		cancel: cancel,
		rules:  ruleStore,
		stop: func() {
			cancel()
			sessions.closeAll()
			_ = conn.Close()
			<-done
		},
	}, nil
}

func (supervisor *Supervisor) proxyTCP(ctx context.Context, downstream net.Conn, rules []agent.RuleConfig) {
	defer func() { _ = downstream.Close() }()
	sourceAddress := downstream.RemoteAddr()
	uploadReader := io.Reader(downstream)
	preface := []byte(nil)

	rule, ok := selectAnyInboundRule(rules)
	if ok {
		if proxyVersion := normalizedProxyProtocol(rule.ProxyProtocolIn); proxyVersion != "" {
			if err := downstream.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				return
			}
			reader := bufio.NewReader(downstream)
			proxyInfo, err := consumeInboundProxyHeader(reader, proxyVersion)
			_ = downstream.SetReadDeadline(time.Time{})
			if err != nil {
				return
			}
			sourceAddress = proxyInfo.sourceAddr()
			uploadReader = reader
		}
	} else {
		selectedRule, selectedPreface, selectedSource, selectedReader, matched := selectSNIRule(downstream, rules)
		if !matched {
			return
		}
		rule = selectedRule
		preface = selectedPreface
		if selectedSource != nil {
			sourceAddress = selectedSource
		}
		uploadReader = selectedReader
	}
	if !ok && rule.ID == "" {
		return
	}
	target, reserved, ok := supervisor.selectTCPTarget(rule, sourceAddress)
	if !ok {
		return
	}
	dialStarted := time.Now()
	upstream, err := dialTCPUpstream(rule.SendIP, targetAddress(target))
	if err != nil {
		if reserved {
			supervisor.metrics.releaseTargetTCPDial(rule.ID, target.ID)
		}
		return
	}
	supervisor.metrics.recordTargetLatency(rule.ID, target.ID, time.Since(dialStarted))
	defer func() { _ = upstream.Close() }()

	supervisor.metrics.addTCPConnection(1)
	if reserved {
		supervisor.metrics.promoteTargetTCPDial(rule.ID, target.ID)
	} else {
		supervisor.metrics.addTargetTCPConnection(rule.ID, target.ID, 1)
	}
	defer func() {
		supervisor.metrics.addTCPConnection(-1)
		supervisor.metrics.addTargetTCPConnection(rule.ID, target.ID, -1)
	}()
	addTargetUpload := func(delta int64) {
		supervisor.metrics.addUpload(delta)
		supervisor.metrics.addTargetUpload(rule.ID, target.ID, delta)
	}
	addTargetDownload := func(delta int64) {
		supervisor.metrics.addDownload(delta)
		supervisor.metrics.addTargetDownload(rule.ID, target.ID, delta)
	}
	if rule.ProxyProtocolOut != "" && rule.ProxyProtocolOut != "NONE" {
		header, err := outboundProxyHeader(rule.ProxyProtocolOut, sourceAddress, upstream.RemoteAddr())
		if err != nil {
			return
		}
		n, err := upstream.Write(header)
		if n > 0 {
			addTargetUpload(int64(n))
		}
		if err != nil {
			return
		}
	}
	if len(preface) > 0 {
		n, err := upstream.Write(preface)
		if n > 0 {
			addTargetUpload(int64(n))
		}
		if err != nil {
			return
		}
	}

	var once sync.Once
	closeBoth := func() {
		_ = downstream.Close()
		_ = upstream.Close()
	}
	connectionDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			once.Do(closeBoth)
		case <-connectionDone:
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(countingWriter{writer: upstream, add: addTargetUpload}, uploadReader)
		once.Do(closeBoth)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(countingWriter{writer: downstream, add: addTargetDownload}, upstream)
		once.Do(closeBoth)
	}()
	wg.Wait()
	close(connectionDone)
}

func (supervisor *Supervisor) proxyUDP(ctx context.Context, sessions *udpSessionTable, clientAddress net.Addr, rules []agent.RuleConfig, payload []byte) {
	rule, ok := selectAnyInboundRule(rules)
	if !ok {
		return
	}
	target, reserved, ok := supervisor.selectUDPTarget(rule, clientAddress)
	if !ok {
		return
	}
	targetID, created, err := sessions.write(ctx, rule, target, clientAddress, payload)
	if reserved {
		if !created {
			supervisor.metrics.releaseTargetUDPSessionReservation(rule.ID, target.ID)
		} else {
			supervisor.metrics.promoteTargetUDPSessionReservation(rule.ID, target.ID)
		}
	} else if created {
		supervisor.metrics.addTargetUDPSession(rule.ID, target.ID, 1)
	}
	if err != nil {
		return
	}
	supervisor.metrics.addUDP(1)
	supervisor.metrics.addUpload(int64(len(payload)))
	supervisor.metrics.addTargetUDP(rule.ID, targetID, 1)
	supervisor.metrics.addTargetUpload(rule.ID, targetID, int64(len(payload)))
}

func newUDPSessionTable(listener net.PacketConn, metrics *metricsCounter) *udpSessionTable {
	return &udpSessionTable{
		listener: listener,
		metrics:  metrics,
		sessions: map[string]*udpSession{},
	}
}

func (table *udpSessionTable) write(ctx context.Context, rule agent.RuleConfig, target agent.TargetEndpoint, clientAddress net.Addr, payload []byte) (string, bool, error) {
	key := udpSessionKey(clientAddress, rule.ID, rule.SendIP)
	upstreamTargetAddress := targetAddress(target)
	table.mu.Lock()
	session := table.sessions[key]
	created := false
	if session != nil && !targetStillConfiguredForRule(rule, session.targetID, session.targetAddress) {
		table.closeSessionLocked(key, session)
		session = nil
	}
	if session == nil {
		upstreamAddress, err := net.ResolveUDPAddr("udp", upstreamTargetAddress)
		if err != nil {
			table.mu.Unlock()
			return "", false, err
		}
		upstream, err := dialUDPUpstream(rule.SendIP, upstreamAddress)
		if err != nil {
			table.mu.Unlock()
			return "", false, err
		}
		session = &udpSession{
			key:           key,
			ruleID:        rule.ID,
			targetID:      target.ID,
			targetAddress: upstreamTargetAddress,
			clientAddress: clientAddress,
			upstream:      upstream,
			lastSeen:      time.Now(),
		}
		table.sessions[key] = session
		go table.readLoop(ctx, session)
		created = true
	}
	session.lastSeen = time.Now()
	upstream := session.upstream
	targetID := session.targetID
	table.mu.Unlock()
	_, err := upstream.Write(payload)
	if err != nil {
		return targetID, created, err
	}
	return targetID, created, nil
}

func targetStillConfiguredForRule(rule agent.RuleConfig, targetID string, targetAddr string) bool {
	switch rule.Upstream.Type {
	case "TARGET":
		return rule.Upstream.Target != nil && rule.Upstream.Target.Enabled && rule.Upstream.Target.ID == targetID && targetAddress(*rule.Upstream.Target) == targetAddr
	case "TARGET_GROUP":
		for _, target := range activeTargetGroupCandidates(rule) {
			if target.ID == targetID && targetAddress(target) == targetAddr {
				return true
			}
		}
	}
	return false
}

func (table *udpSessionTable) readLoop(ctx context.Context, session *udpSession) {
	buffer := make([]byte, 65535)
	for {
		n, err := session.upstream.Read(buffer)
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			if _, err := table.listener.WriteTo(buffer[:n], session.clientAddress); err == nil {
				table.metrics.addDownload(int64(n))
				table.metrics.addTargetDownload(session.ruleID, session.targetID, int64(n))
			}
		}
	}
}

func (table *udpSessionTable) closeIdle(now time.Time) {
	table.mu.Lock()
	defer table.mu.Unlock()
	for key, session := range table.sessions {
		if now.Sub(session.lastSeen) <= udpSessionIdleTimeout {
			continue
		}
		table.closeSessionLocked(key, session)
	}
}

func (table *udpSessionTable) closeAll() {
	table.mu.Lock()
	defer table.mu.Unlock()
	for key, session := range table.sessions {
		table.closeSessionLocked(key, session)
	}
}

func (table *udpSessionTable) closeSessionLocked(key string, session *udpSession) {
	_ = session.upstream.Close()
	delete(table.sessions, key)
	table.metrics.addTargetUDPSession(session.ruleID, session.targetID, -1)
}

func udpSessionKey(clientAddress net.Addr, ruleID string, sendIP string) string {
	return clientAddress.String() + "\x00" + ruleID + "\x00" + strings.TrimSpace(sendIP)
}

func groupRulesByListener(rules []agent.RuleConfig) (map[listenerKey][]agent.RuleConfig, error) {
	grouped := map[listenerKey][]agent.RuleConfig{}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if normalizedForwardingType(rule) != domain.ForwardingTypeDirect {
			return nil, fmt.Errorf("unsupported forwarding type %s", rule.ForwardingType)
		}
		for _, protocol := range listenerProtocols(rule.Protocol) {
			key := listenerKey{listenIP: rule.ListenIP, protocol: protocol, port: rule.Port}
			grouped[key] = append(grouped[key], rule)
		}
	}
	return grouped, nil
}

func normalizedForwardingType(rule agent.RuleConfig) domain.ForwardingType {
	if rule.ForwardingType == "" {
		return domain.ForwardingTypeDirect
	}
	return rule.ForwardingType
}

func listenerProtocols(protocol domain.Protocol) []domain.Protocol {
	if protocol == domain.ProtocolTCPUDP {
		return []domain.Protocol{domain.ProtocolTCP, domain.ProtocolUDP}
	}
	return []domain.Protocol{protocol}
}

func selectAnyInboundRule(rules []agent.RuleConfig) (agent.RuleConfig, bool) {
	for _, rule := range rules {
		if rule.MatchType == "ANY_INBOUND" {
			return rule, true
		}
	}
	return agent.RuleConfig{}, false
}

func selectSNIRule(conn net.Conn, rules []agent.RuleConfig) (agent.RuleConfig, []byte, net.Addr, io.Reader, bool) {
	if !hasSNIRules(rules) {
		return agent.RuleConfig{}, nil, nil, nil, false
	}
	proxyVersion, ok := sniInboundProxyProtocol(rules)
	if !ok {
		return agent.RuleConfig{}, nil, nil, nil, false
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return agent.RuleConfig{}, nil, nil, nil, false
	}
	reader := bufio.NewReader(conn)
	var sourceAddress net.Addr
	if proxyVersion != "" {
		proxyInfo, err := consumeInboundProxyHeader(reader, proxyVersion)
		if err != nil {
			_ = conn.SetReadDeadline(time.Time{})
			return agent.RuleConfig{}, nil, nil, nil, false
		}
		sourceAddress = proxyInfo.sourceAddr()
	}
	preface, hostname, err := readTLSClientHelloSNI(reader)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return agent.RuleConfig{}, nil, nil, nil, false
	}
	for _, rule := range rules {
		if rule.MatchType == "TLS_SNI" && strings.EqualFold(rule.SNIHostname, hostname) {
			return rule, preface, sourceAddress, reader, true
		}
	}
	return agent.RuleConfig{}, nil, nil, nil, false
}

func hasSNIRules(rules []agent.RuleConfig) bool {
	for _, rule := range rules {
		if rule.MatchType == "TLS_SNI" {
			return true
		}
	}
	return false
}

func sniInboundProxyProtocol(rules []agent.RuleConfig) (string, bool) {
	versionSet := false
	version := ""
	for _, rule := range rules {
		if rule.MatchType != "TLS_SNI" {
			continue
		}
		ruleVersion := normalizedProxyProtocol(rule.ProxyProtocolIn)
		if !versionSet {
			version = ruleVersion
			versionSet = true
			continue
		}
		if ruleVersion != version {
			return "", false
		}
	}
	return version, versionSet
}

func normalizedProxyProtocol(version string) string {
	if version == "" || version == "NONE" {
		return ""
	}
	return version
}

func readTLSClientHelloSNI(reader io.Reader) ([]byte, string, error) {
	var preface []byte
	var handshake []byte
	expectedHandshakeBytes := 0
	for len(preface) < maxTLSClientHelloBytes {
		header := make([]byte, 5)
		if _, err := io.ReadFull(reader, header); err != nil {
			return preface, "", err
		}
		preface = append(preface, header...)
		if header[0] != 0x16 {
			return preface, "", errors.New("not a tls handshake record")
		}
		recordLength := int(binary.BigEndian.Uint16(header[3:5]))
		if recordLength <= 0 || len(preface)+recordLength > maxTLSClientHelloBytes {
			return preface, "", errors.New("tls client hello too large")
		}
		body := make([]byte, recordLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			return preface, "", err
		}
		preface = append(preface, body...)
		handshake = append(handshake, body...)
		if len(handshake) >= 4 && expectedHandshakeBytes == 0 {
			if handshake[0] != 0x01 {
				return preface, "", errors.New("not a client hello")
			}
			handshakeLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
			expectedHandshakeBytes = 4 + handshakeLength
			if expectedHandshakeBytes > maxTLSClientHelloBytes {
				return preface, "", errors.New("tls client hello too large")
			}
		}
		if expectedHandshakeBytes > 0 && len(handshake) >= expectedHandshakeBytes {
			hostname, err := parseClientHelloSNI(handshake[:expectedHandshakeBytes])
			if err != nil {
				return preface, "", err
			}
			return preface, hostname, nil
		}
	}
	return preface, "", errors.New("tls client hello too large")
}

func parseClientHelloSNI(handshake []byte) (string, error) {
	if len(handshake) < 4 || handshake[0] != 0x01 {
		return "", errors.New("not a client hello")
	}
	handshakeLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if handshakeLength > len(handshake)-4 {
		return "", errors.New("client hello truncated")
	}
	cursor := 4
	if cursor+2+32 > len(handshake) {
		return "", errors.New("client hello missing random")
	}
	cursor += 2 + 32
	if cursor+1 > len(handshake) {
		return "", errors.New("client hello missing session id")
	}
	sessionIDLength := int(handshake[cursor])
	cursor++
	if cursor+sessionIDLength+2 > len(handshake) {
		return "", errors.New("client hello session id truncated")
	}
	cursor += sessionIDLength
	cipherSuiteLength := int(binary.BigEndian.Uint16(handshake[cursor : cursor+2]))
	cursor += 2
	if cursor+cipherSuiteLength+1 > len(handshake) {
		return "", errors.New("client hello cipher suites truncated")
	}
	cursor += cipherSuiteLength
	compressionMethodsLength := int(handshake[cursor])
	cursor++
	if cursor+compressionMethodsLength+2 > len(handshake) {
		return "", errors.New("client hello compression methods truncated")
	}
	cursor += compressionMethodsLength
	extensionsLength := int(binary.BigEndian.Uint16(handshake[cursor : cursor+2]))
	cursor += 2
	if cursor+extensionsLength > len(handshake) {
		return "", errors.New("client hello extensions truncated")
	}
	extensionsEnd := cursor + extensionsLength
	for cursor+4 <= extensionsEnd {
		extensionType := binary.BigEndian.Uint16(handshake[cursor : cursor+2])
		extensionLength := int(binary.BigEndian.Uint16(handshake[cursor+2 : cursor+4]))
		cursor += 4
		if cursor+extensionLength > extensionsEnd {
			return "", errors.New("client hello extension truncated")
		}
		if extensionType == 0x0000 {
			return parseSNIExtension(handshake[cursor : cursor+extensionLength])
		}
		cursor += extensionLength
	}
	return "", errors.New("client hello missing sni")
}

func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", errors.New("sni extension missing list length")
	}
	listLength := int(binary.BigEndian.Uint16(data[:2]))
	cursor := 2
	if cursor+listLength > len(data) {
		return "", errors.New("sni extension list truncated")
	}
	end := cursor + listLength
	for cursor+3 <= end {
		nameType := data[cursor]
		nameLength := int(binary.BigEndian.Uint16(data[cursor+1 : cursor+3]))
		cursor += 3
		if cursor+nameLength > end {
			return "", errors.New("sni extension name truncated")
		}
		if nameType == 0 {
			return string(data[cursor : cursor+nameLength]), nil
		}
		cursor += nameLength
	}
	return "", errors.New("sni extension missing dns name")
}

func consumeInboundProxyHeader(reader *bufio.Reader, version string) (ProxyInfo, error) {
	switch version {
	case "V1":
		line, err := readProxyProtocolV1Line(reader)
		if err != nil {
			return ProxyInfo{}, err
		}
		info, _, err := ParseProxyHeader(line)
		if err != nil {
			return ProxyInfo{}, err
		}
		if info.Version != ProxyProtocolV1 {
			return ProxyInfo{}, errors.New("missing proxy protocol v1 header")
		}
		return info, nil
	case "V2":
		header := make([]byte, 16)
		if _, err := io.ReadFull(reader, header); err != nil {
			return ProxyInfo{}, err
		}
		length := int(binary.BigEndian.Uint16(header[14:16]))
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return ProxyInfo{}, err
		}
		info, _, err := ParseProxyHeader(append(header, body...))
		if err != nil {
			return ProxyInfo{}, err
		}
		if info.Version != ProxyProtocolV2 {
			return ProxyInfo{}, errors.New("missing proxy protocol v2 header")
		}
		return info, nil
	default:
		return ProxyInfo{}, errors.New("unsupported inbound proxy protocol")
	}
}

func readProxyProtocolV1Line(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, maxProxyProtocolV1LineLen)
	for {
		if len(line) >= maxProxyProtocolV1LineLen {
			return nil, errors.New("proxy protocol v1 header too long")
		}
		next, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		line = append(line, next)
		if next == '\n' {
			return line, nil
		}
	}
}

func outboundProxyHeader(version string, source net.Addr, destination net.Addr) ([]byte, error) {
	sourceHost, sourcePort := splitAddr(source)
	destinationHost, destinationPort := splitAddr(destination)
	sourceTCPAddr := &net.TCPAddr{IP: sourceHost, Port: sourcePort}
	destinationTCPAddr := &net.TCPAddr{IP: destinationHost, Port: destinationPort}
	switch version {
	case "V1":
		return proxyproto.HeaderProxyFromAddrs(1, sourceTCPAddr, destinationTCPAddr).Format()
	case "V2":
		return proxyproto.HeaderProxyFromAddrs(2, sourceTCPAddr, destinationTCPAddr).Format()
	default:
		return nil, errors.New("unsupported outbound proxy protocol")
	}
}

func splitAddr(address net.Addr) (net.IP, int) {
	host, portText, err := net.SplitHostPort(address.String())
	if err != nil {
		return net.IPv4(127, 0, 0, 1), 0
	}
	port, _ := strconv.Atoi(portText)
	ip := net.ParseIP(host)
	if ip == nil {
		ip = net.IPv4(127, 0, 0, 1)
	}
	return ip, port
}

func (info ProxyInfo) sourceAddr() net.Addr {
	return &net.TCPAddr{IP: info.SourceIP, Port: info.SourcePort}
}

func targetAddress(target agent.TargetEndpoint) string {
	return net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
}

type countingWriter struct {
	writer io.Writer
	add    func(int64)
}

func (writer countingWriter) Write(payload []byte) (int, error) {
	n, err := writer.writer.Write(payload)
	if n > 0 {
		writer.add(int64(n))
	}
	return n, err
}
