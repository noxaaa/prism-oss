package dataplane

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type NFTablesBackend struct {
	options Options
	mu      sync.Mutex
	last    map[string]managedTrafficSnapshot
}

var nftCounterPattern = regexp.MustCompile(`counter packets ([0-9]+) bytes ([0-9]+).*comment "prism rule=([^" ]+)`)

func NewNFTablesBackend(options Options) *NFTablesBackend {
	if strings.TrimSpace(options.NFTPath) == "" {
		options.NFTPath = "nft"
	}
	return &NFTablesBackend{options: options}
}

func (backend *NFTablesBackend) Name() string {
	return ModeNFTables
}

func (backend *NFTablesBackend) Capabilities() Capabilities {
	return Capabilities{TCP: true, UDP: true, KernelL4: true}
}

func (backend *NFTablesBackend) Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error {
	paths := backend.paths()
	if len(snapshot.Rules) == 0 {
		return backend.cleanupTable(ctx, nil)
	}
	for _, rule := range snapshot.Rules {
		if !nftablesSupportsRule(rule) {
			return configApplyError("nftables does not support this rule", unsupportedRuleError(rule, ModeNFTables, "unsupported nftables rule"))
		}
	}
	if err := checkNFTablesListenersAvailable(snapshot.Rules); err != nil {
		return err
	}
	if backend.options.ExternalBackends {
		if err := checkNFTablesKernelForwarding(snapshot.Rules); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(paths.configDir, 0o700); err != nil {
		return err
	}
	rendered, err := renderNFTablesConfig(snapshot, backend.tableName(), backend.tableExists(ctx))
	if err != nil {
		return err
	}
	configPath := filepath.Join(paths.configDir, fmt.Sprintf("%d.nft", snapshot.ConfigVersion))
	if err := os.WriteFile(configPath, []byte(rendered), 0o600); err != nil {
		return err
	}
	output, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.NFTPath, "-f", configPath)
	if err != nil {
		return configApplyError(strings.TrimSpace(string(output)), nftablesApplyDetails(snapshot.Rules, strings.TrimSpace(string(output)))...)
	}
	return nil
}

func (backend *NFTablesBackend) Status(context.Context) Status {
	return Status{Dataplane: ModeNFTables, Available: true, Health: "UNKNOWN"}
}

func (backend *NFTablesBackend) AgentMetrics() agent.MetricsPayload {
	current := backend.readTrafficCounters(context.Background())
	if current == nil {
		return agent.MetricsPayload{}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	deltas := managedTrafficDeltas(current, backend.last)
	backend.last = current
	return agent.MetricsPayload{TrafficDeltas: deltas}
}

func (backend *NFTablesBackend) CleanupOwnedState(ctx context.Context) error {
	return backend.cleanupTable(ctx, nil)
}

func (backend *NFTablesBackend) Close() error {
	return nil
}

func (backend *NFTablesBackend) cleanupTable(ctx context.Context, rules []agent.RuleConfig) error {
	output, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.NFTPath, "destroy", "table", "inet", backend.tableName())
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" && nftTableMissing(message) {
			return nil
		}
		if message == "" {
			message = err.Error()
		}
		if len(rules) == 0 {
			rules = []agent.RuleConfig{{
				ID:        "nftables-cleanup",
				Protocol:  domain.ProtocolTCPUDP,
				ListenIP:  "0.0.0.0",
				Port:      0,
				MatchType: string(domain.MatchTypeAnyInbound),
			}}
		}
		return configApplyError(message, nftablesApplyDetails(rules, message)...)
	}
	return nil
}

func nftTableMissing(message string) bool {
	value := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(value, "no such file") ||
		strings.Contains(value, "does not exist")
}

func (backend *NFTablesBackend) tableExists(ctx context.Context) bool {
	_, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.NFTPath, "list", "table", "inet", backend.tableName())
	return err == nil
}

func (backend *NFTablesBackend) readTrafficCounters(ctx context.Context) map[string]managedTrafficSnapshot {
	output, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.NFTPath, "list", "table", "inet", backend.tableName())
	if err != nil {
		return nil
	}
	result := map[string]managedTrafficSnapshot{}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, " dnat ") {
			continue
		}
		matches := nftCounterPattern.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}
		ruleID := matches[3]
		snapshot := result[ruleID]
		packetsValue := parseInt64(matches[1])
		bytesValue := parseInt64(matches[2])
		snapshot.UploadBytes += bytesValue
		if strings.Contains(line, " udp dport ") {
			snapshot.UDPPackets += packetsValue
		}
		result[ruleID] = snapshot
	}
	return result
}

type nftablesPaths struct {
	configDir string
}

func (backend *NFTablesBackend) paths() nftablesPaths {
	return nftablesPaths{configDir: filepath.Join(backend.options.StateDir, "nftables")}
}

func (backend *NFTablesBackend) tableName() string {
	return nftablesTableNameForInstance(backend.options.InstanceID)
}

func nftablesSupportsRule(rule agent.RuleConfig) bool {
	if strings.TrimSpace(rule.SendIP) != "" {
		return false
	}
	if rule.MatchType != string(domain.MatchTypeAnyInbound) {
		return false
	}
	if strings.TrimSpace(rule.ProxyProtocolIn) != "" && strings.TrimSpace(rule.ProxyProtocolIn) != "NONE" {
		return false
	}
	if strings.TrimSpace(rule.ProxyProtocolOut) != "" && strings.TrimSpace(rule.ProxyProtocolOut) != "NONE" {
		return false
	}
	if rule.Upstream.Type != "TARGET" || rule.Upstream.Target == nil {
		return false
	}
	if !rule.Upstream.Target.Enabled {
		return true
	}
	targetIP := net.ParseIP(rule.Upstream.Target.Host)
	if targetIP == nil {
		return false
	}
	if targetIP.IsLoopback() {
		return false
	}
	if nftListenAddressIsLoopback(rule.ListenIP) {
		return false
	}
	listenFamily := nftListenAddressFamily(rule.ListenIP)
	return listenFamily == "" || listenFamily == nftIPFamily(targetIP)
}

func renderNFTablesConfig(snapshot agent.ConfigSnapshot, tableName string, replaceExisting bool) (string, error) {
	rules := append([]agent.RuleConfig(nil), snapshot.Rules...)
	sort.SliceStable(rules, func(i int, j int) bool {
		if rules[i].Protocol != rules[j].Protocol {
			return rules[i].Protocol < rules[j].Protocol
		}
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].ID < rules[j].ID
	})
	var builder strings.Builder
	if replaceExisting {
		builder.WriteString("delete table inet ")
		builder.WriteString(tableName)
		builder.WriteString("\n")
	}
	builder.WriteString("table inet ")
	builder.WriteString(tableName)
	builder.WriteString(" {\n")
	builder.WriteString("  chain prerouting {\n")
	builder.WriteString("    type nat hook prerouting priority dstnat; policy accept;\n")
	for _, rule := range rules {
		if rule.Upstream.Target == nil || !rule.Upstream.Target.Enabled {
			continue
		}
		target := rule.Upstream.Target
		ip := net.ParseIP(target.Host)
		if ip == nil {
			return "", configApplyError("nftables target must be a literal IP", unsupportedRuleError(rule, ModeNFTables, "target host must be a literal IP"))
		}
		if listenFamily := nftListenAddressFamily(rule.ListenIP); listenFamily != "" && listenFamily != nftIPFamily(ip) {
			return "", configApplyError("nftables cannot DNAT across address families", unsupportedRuleError(rule, ModeNFTables, "listen address and target address must use the same IP family"))
		}
		family := "ip"
		if ip.To4() == nil {
			family = "ip6"
		}
		for _, protocol := range nftRuleProtocols(rule.Protocol) {
			builder.WriteString("    ")
			if listenMatch := nftListenMatch(rule.ListenIP); listenMatch != "" {
				builder.WriteString(listenMatch)
				builder.WriteString(" ")
			}
			builder.WriteString(protocol)
			builder.WriteString(" dport ")
			builder.WriteString(strconv.Itoa(rule.Port))
			builder.WriteString(" counter meta mark set 0x70726973 dnat ")
			builder.WriteString(family)
			builder.WriteString(" to ")
			builder.WriteString(nftDNATTargetHost(ip, target.Host, target.Port))
			if target.Port > 0 {
				builder.WriteString(":")
				builder.WriteString(strconv.Itoa(target.Port))
			}
			builder.WriteString(" comment \"prism rule=")
			builder.WriteString(sanitizeComment(rule.ID))
			builder.WriteString(" version=")
			builder.WriteString(strconv.Itoa(snapshot.ConfigVersion))
			builder.WriteString(" hash=")
			builder.WriteString(sanitizeComment(snapshot.ConfigHash))
			builder.WriteString("\"\n")
		}
	}
	builder.WriteString("  }\n")
	builder.WriteString("  chain postrouting {\n")
	builder.WriteString("    type nat hook postrouting priority srcnat; policy accept;\n")
	for _, rule := range rules {
		if rule.Upstream.Target == nil || !rule.Upstream.Target.Enabled {
			continue
		}
		target := rule.Upstream.Target
		ip := net.ParseIP(target.Host)
		if ip == nil {
			return "", configApplyError("nftables target must be a literal IP", unsupportedRuleError(rule, ModeNFTables, "target host must be a literal IP"))
		}
		family := "ip"
		if ip.To4() == nil {
			family = "ip6"
		}
		for _, protocol := range nftRuleProtocols(rule.Protocol) {
			builder.WriteString("    ")
			builder.WriteString(family)
			builder.WriteString(" daddr ")
			builder.WriteString(target.Host)
			builder.WriteString(" ")
			builder.WriteString(protocol)
			builder.WriteString(" dport ")
			builder.WriteString(strconv.Itoa(target.Port))
			builder.WriteString(" meta mark 0x70726973 masquerade comment \"prism postrouting-rule=")
			builder.WriteString(sanitizeComment(rule.ID))
			builder.WriteString(" version=")
			builder.WriteString(strconv.Itoa(snapshot.ConfigVersion))
			builder.WriteString(" hash=")
			builder.WriteString(sanitizeComment(snapshot.ConfigHash))
			builder.WriteString("\"\n")
		}
	}
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}

func nftDNATTargetHost(ip net.IP, host string, port int) string {
	if ip.To4() == nil && port > 0 {
		return "[" + host + "]"
	}
	return host
}

var (
	readSysctlFile = os.ReadFile
	currentGOOS    = runtime.GOOS
)

func checkNFTablesKernelForwarding(rules []agent.RuleConfig) error {
	if currentGOOS != "linux" {
		return nil
	}
	needsIPv4 := false
	needsIPv6 := false
	for _, rule := range rules {
		if rule.Upstream.Target == nil || !rule.Upstream.Target.Enabled {
			continue
		}
		ip := net.ParseIP(rule.Upstream.Target.Host)
		if ip == nil || !nftTargetRequiresForwarding(ip) {
			continue
		}
		if ip.To4() == nil {
			needsIPv6 = true
		} else {
			needsIPv4 = true
		}
	}
	if needsIPv4 {
		if err := requireSysctlEnabled("/proc/sys/net/ipv4/ip_forward", "IPv4 forwarding is disabled", rules); err != nil {
			return err
		}
	}
	if needsIPv6 {
		if err := requireSysctlEnabled("/proc/sys/net/ipv6/conf/all/forwarding", "IPv6 forwarding is disabled", rules); err != nil {
			return err
		}
	}
	return nil
}

func nftTargetRequiresForwarding(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsUnspecified()
}

func nftListenAddressIsLoopback(listenIP string) bool {
	value := strings.TrimSpace(listenIP)
	if value == "" || value == "0.0.0.0" || value == "::" {
		return false
	}
	ip := net.ParseIP(value)
	return ip != nil && ip.IsLoopback()
}

func requireSysctlEnabled(path string, message string, rules []agent.RuleConfig) error {
	data, err := readSysctlFile(path)
	if err != nil {
		return configApplyError(message+": "+err.Error(), unsupportedBackendDetails(rules, ModeNFTables, message)...)
	}
	if strings.TrimSpace(string(data)) != "1" {
		return configApplyError(message, unsupportedBackendDetails(rules, ModeNFTables, message)...)
	}
	return nil
}

func nftRuleProtocols(protocol domain.Protocol) []string {
	switch protocol {
	case domain.ProtocolTCPUDP:
		return []string{"tcp", "udp"}
	case domain.ProtocolUDP:
		return []string{"udp"}
	default:
		return []string{"tcp"}
	}
}

func nftListenMatch(listenIP string) string {
	value := strings.TrimSpace(listenIP)
	if value == "" || value == "0.0.0.0" {
		return "fib daddr type local ip daddr 0.0.0.0/0"
	}
	if value == "::" {
		return "fib daddr type local ip6 daddr ::/0"
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	if ip.To4() == nil {
		return "ip6 daddr " + value
	}
	return "ip daddr " + value
}

func nftListenAddressFamily(listenIP string) string {
	value := strings.TrimSpace(listenIP)
	if value == "" || value == "0.0.0.0" {
		return "ip"
	}
	if value == "::" {
		return "ip6"
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return nftIPFamily(ip)
}

func nftIPFamily(ip net.IP) string {
	if ip.To4() == nil {
		return "ip6"
	}
	return "ip"
}

func nftablesApplyDetails(rules []agent.RuleConfig, message string) []agent.ConfigApplyErrorDetail {
	details := make([]agent.ConfigApplyErrorDetail, 0, len(rules))
	for _, rule := range rules {
		detail := unsupportedRuleError(rule, ModeNFTables, message)
		detail.Code = ErrorNFTablesLocked
		details = append(details, detail)
	}
	return details
}

func checkNFTablesListenersAvailable(rules []agent.RuleConfig) error {
	grouped := make(map[listenerKey][]agent.RuleConfig)
	for _, rule := range rules {
		if rule.Upstream.Target == nil || !rule.Upstream.Target.Enabled {
			continue
		}
		for _, key := range listenerKeysForRule(rule) {
			grouped[key] = append(grouped[key], rule)
		}
	}
	keys := make([]listenerKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		return listenerKeyString(keys[i]) < listenerKeyString(keys[j])
	})
	for _, key := range keys {
		if err := probeListenerAvailable(key); err != nil {
			return configApplyError(err.Error(),
				listenerError(ErrorListenerOwnedByExternalProcess, key, grouped[key], ModeNFTables, "external", err.Error()),
			)
		}
	}
	return nil
}

func probeListenerAvailable(key listenerKey) error {
	switch key.protocol {
	case domain.ProtocolUDP:
		addr, err := net.ResolveUDPAddr(listenerNetwork("udp", key.listenIP), net.JoinHostPort(key.listenIP, strconv.Itoa(key.port)))
		if err != nil {
			return err
		}
		conn, err := net.ListenUDP(listenerNetwork("udp", key.listenIP), addr)
		if err != nil {
			return err
		}
		return conn.Close()
	default:
		addr, err := net.ResolveTCPAddr(listenerNetwork("tcp", key.listenIP), net.JoinHostPort(key.listenIP, strconv.Itoa(key.port)))
		if err != nil {
			return err
		}
		listener, err := net.ListenTCP(listenerNetwork("tcp", key.listenIP), addr)
		if err != nil {
			return err
		}
		return listener.Close()
	}
}

func listenerNetwork(base string, listenIP string) string {
	ip := net.ParseIP(strings.Trim(strings.TrimSpace(listenIP), "[]"))
	if ip == nil {
		return base
	}
	if ip.To4() == nil {
		return base + "6"
	}
	return base + "4"
}
