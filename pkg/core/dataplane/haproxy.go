package dataplane

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type HAProxyBackend struct {
	options Options
	mu      sync.Mutex
	last    map[string]managedTrafficSnapshot
	ruleIDs map[string]string
}

var haproxySignalProcess = func(process *os.Process, signal os.Signal) error {
	return process.Signal(signal)
}

var haproxyProcessLooksManaged = func(backend *HAProxyBackend, pid int) bool {
	return backend.pidLooksLikeManagedHAProxy(pid)
}

func NewHAProxyBackend(options Options) *HAProxyBackend {
	if strings.TrimSpace(options.HAProxyPath) == "" {
		options.HAProxyPath = filepath.Join(options.InstallDir, "current", "dataplane", "haproxy", "haproxy")
	}
	return &HAProxyBackend{options: options}
}

func (backend *HAProxyBackend) Name() string {
	return ModeHAProxy
}

func (backend *HAProxyBackend) Capabilities() Capabilities {
	return Capabilities{
		TCP:              true,
		TLSSNI:           true,
		ProxyProtocolIn:  true,
		ProxyProtocolOut: true,
		TargetGroup:      true,
		HostnameTarget:   true,
	}
}

func (backend *HAProxyBackend) Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error {
	if len(snapshot.Rules) == 0 {
		return backend.stopOwnedProcess()
	}
	if _, err := os.Stat(backend.options.HAProxyPath); err != nil {
		return configApplyError("Prism managed HAProxy binary is unavailable", unsupportedBackendDetails(snapshot.Rules, ModeHAProxy, "HAProxy binary is not installed")...)
	}
	nextRuleIDs := haproxyRuleNames(snapshot.Rules)
	rendered, err := renderHAProxyConfig(snapshot, backend.paths())
	if err != nil {
		return err
	}
	paths := backend.paths()
	if err := os.MkdirAll(paths.configDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		return err
	}
	tmpConfig := filepath.Join(paths.configDir, fmt.Sprintf(".%d.cfg.tmp", snapshot.ConfigVersion))
	activeConfig := filepath.Join(paths.configDir, "active.cfg")
	if err := os.WriteFile(tmpConfig, []byte(rendered), 0o600); err != nil {
		return err
	}
	validateOutput, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.HAProxyPath, "-c", "-f", tmpConfig)
	if err != nil {
		return configApplyError(strings.TrimSpace(string(validateOutput)), haproxyValidateDetails(snapshot.Rules, strings.TrimSpace(string(validateOutput)))...)
	}
	if err := os.Rename(tmpConfig, activeConfig); err != nil {
		return err
	}
	args := []string{"-D", "-f", activeConfig, "-p", paths.pidFile}
	if oldPID := backend.managedHAProxyPID(); oldPID != "" {
		if _, err := os.Stat(paths.socketFile); err == nil {
			args = append(args, "-x", paths.socketFile)
		}
		args = append(args, "-sf", oldPID)
	}
	startOutput, err := backend.options.CommandRunner.CombinedOutput(ctx, backend.options.HAProxyPath, args...)
	if err != nil {
		message := strings.TrimSpace(string(startOutput))
		if message == "" {
			message = err.Error()
		}
		return configApplyError(message, unsupportedBackendDetails(snapshot.Rules, ModeHAProxy, message)...)
	}
	backend.rememberRuleNames(nextRuleIDs)
	return nil
}

func (backend *HAProxyBackend) Status(context.Context) Status {
	status := Status{Dataplane: ModeHAProxy, Health: "UNKNOWN"}
	if _, err := os.Stat(backend.options.HAProxyPath); err != nil {
		status.Available = false
		status.Error = err.Error()
		return status
	}
	status.Available = true
	status.Health = "READY"
	return status
}

func (backend *HAProxyBackend) AgentMetrics() agent.MetricsPayload {
	current := backend.readTrafficCounters()
	if current == nil {
		return agent.MetricsPayload{}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	deltas := managedTrafficDeltas(current, backend.last)
	backend.last = current
	return agent.MetricsPayload{TrafficDeltas: deltas}
}

func (backend *HAProxyBackend) CleanupOwnedState(context.Context) error {
	return backend.stopOwnedProcess()
}

func (backend *HAProxyBackend) Close() error {
	return nil
}

func (backend *HAProxyBackend) stopOwnedProcess() error {
	paths := backend.paths()
	pid := readPID(paths.pidFile)
	if pid == "" {
		backend.removeRuntimeFiles()
		return nil
	}
	value, err := strconv.Atoi(pid)
	if err != nil || value <= 0 {
		backend.removeRuntimeFiles()
		return nil
	}
	process, err := os.FindProcess(value)
	if err != nil {
		backend.removeRuntimeFiles()
		return nil
	}
	if !haproxyProcessLooksManaged(backend, value) {
		backend.removeRuntimeFiles()
		return nil
	}
	if err := haproxySignalProcess(process, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) || !processAlive(pid) {
			backend.removeRuntimeFiles()
			return nil
		}
		return fmt.Errorf("stop managed HAProxy pid %d: %w", value, err)
	}
	if err := backend.waitForManagedHAProxyExit(value); err != nil {
		return err
	}
	backend.removeRuntimeFiles()
	return nil
}

func (backend *HAProxyBackend) managedHAProxyPID() string {
	pid := readPID(backend.paths().pidFile)
	if pid == "" {
		return ""
	}
	value, err := strconv.Atoi(pid)
	if err != nil || value <= 0 {
		return ""
	}
	if !haproxyProcessLooksManaged(backend, value) {
		return ""
	}
	return pid
}

func (backend *HAProxyBackend) pidLooksLikeManagedHAProxy(pid int) bool {
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err == nil && backend.pathIsManagedHAProxyExecutable(exe) {
		return true
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	for _, value := range strings.Split(string(cmdline), "\x00") {
		if backend.pathIsManagedHAProxyExecutable(value) {
			return true
		}
	}
	return false
}

func (backend *HAProxyBackend) waitForManagedHAProxyExit(pid int) error {
	deadline := time.Now().Add(3 * time.Second)
	pidText := strconv.Itoa(pid)
	for time.Now().Before(deadline) {
		if !processAlive(pidText) || !backend.pidLooksLikeManagedHAProxy(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for managed HAProxy pid %d to exit", pid)
}

func (backend *HAProxyBackend) removeRuntimeFiles() {
	paths := backend.paths()
	_ = os.Remove(paths.pidFile)
	_ = os.Remove(paths.socketFile)
}

func (backend *HAProxyBackend) pathIsManagedHAProxyExecutable(path string) bool {
	actual := strings.TrimSpace(path)
	if actual == "" {
		return false
	}
	expectedWrapper, err := filepath.EvalSymlinks(backend.options.HAProxyPath)
	if err != nil {
		expectedWrapper = backend.options.HAProxyPath
	}
	expectedBinary, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(backend.options.HAProxyPath), "haproxy.bin"))
	if err != nil {
		expectedBinary = filepath.Join(filepath.Dir(backend.options.HAProxyPath), "haproxy.bin")
	}
	actualResolved, err := filepath.EvalSymlinks(actual)
	if err != nil {
		actualResolved = actual
	}
	if actualResolved == expectedWrapper || actualResolved == expectedBinary {
		return true
	}
	return backend.pathIsInstalledHAProxyExecutable(actual) || backend.pathIsInstalledHAProxyExecutable(actualResolved)
}

func (backend *HAProxyBackend) pathIsInstalledHAProxyExecutable(path string) bool {
	value := filepath.Clean(strings.TrimSpace(path))
	if value == "." {
		return false
	}
	base := filepath.Base(value)
	if base != "haproxy" && base != "haproxy.bin" {
		return false
	}
	installDir := filepath.Clean(backend.options.InstallDir)
	if installDir == "." || installDir == "" {
		return false
	}
	if value != installDir && !strings.HasPrefix(value, installDir+string(os.PathSeparator)) {
		return false
	}
	normalized := filepath.ToSlash(value)
	return strings.Contains(normalized, "/dataplane/haproxy/")
}

type haproxyPaths struct {
	configDir  string
	runDir     string
	pidFile    string
	socketFile string
}

func (backend *HAProxyBackend) paths() haproxyPaths {
	configDir := filepath.Join(backend.options.StateDir, "haproxy")
	runDir := filepath.Join(backend.options.RunDir, "dataplane", sanitizeName(backend.options.InstanceID), "haproxy")
	return haproxyPaths{
		configDir:  configDir,
		runDir:     runDir,
		pidFile:    filepath.Join(runDir, "haproxy.pid"),
		socketFile: filepath.Join(runDir, "haproxy.sock"),
	}
}

func (backend *HAProxyBackend) readTrafficCounters() map[string]managedTrafficSnapshot {
	paths := backend.paths()
	conn, err := net.DialTimeout("unix", paths.socketFile, time.Second)
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(conn, "show stat\n"); err != nil {
		return nil
	}
	reader := csv.NewReader(conn)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "# ")
	}
	indexes := csvHeaderIndexes(header)
	result := map[string]managedTrafficSnapshot{}
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if csvValue(row, indexes, "svname") != "BACKEND" {
			continue
		}
		backendName := csvValue(row, indexes, "pxname")
		if !strings.HasPrefix(backendName, "be_") {
			continue
		}
		ruleID := backend.ruleIDForBackend(backendName)
		if ruleID == "" {
			continue
		}
		addManagedTrafficSnapshot(result, ruleID, managedTrafficSnapshot{
			UploadBytes:         parseInt64(csvValue(row, indexes, "bin")),
			DownloadBytes:       parseInt64(csvValue(row, indexes, "bout")),
			TCPConnectionEvents: parseInt64(csvValue(row, indexes, "stot")),
		})
	}
	return result
}

func haproxyRuleNames(rules []agent.RuleConfig) map[string]string {
	ruleIDs := make(map[string]string, len(rules))
	for _, rule := range rules {
		ruleIDs[haproxyBackendName(rule)] = rule.ID
	}
	return ruleIDs
}

func (backend *HAProxyBackend) rememberRuleNames(ruleIDs map[string]string) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.ruleIDs = ruleIDs
}

func (backend *HAProxyBackend) ruleIDForBackend(backendName string) string {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.ruleIDs == nil {
		return strings.TrimPrefix(backendName, "be_")
	}
	return backend.ruleIDs[backendName]
}

func haproxySupportsRule(rule agent.RuleConfig) bool {
	if rule.Protocol != domain.ProtocolTCP {
		return false
	}
	switch domain.MatchType(rule.MatchType) {
	case domain.MatchTypeAnyInbound, domain.MatchTypeTLSSNI:
	default:
		return false
	}
	return upstreamTargets(rule) != nil
}

func renderHAProxyConfig(snapshot agent.ConfigSnapshot, paths haproxyPaths) (string, error) {
	grouped := map[listenerKey][]agent.RuleConfig{}
	for _, rule := range snapshot.Rules {
		if !haproxySupportsRule(rule) {
			return "", configApplyError("HAProxy does not support this rule", unsupportedRuleError(rule, ModeHAProxy, "unsupported HAProxy rule"))
		}
		key := listenerKeyForRule(rule)
		grouped[key] = append(grouped[key], rule)
	}
	keys := make([]listenerKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].protocol != keys[j].protocol {
			return keys[i].protocol < keys[j].protocol
		}
		if keys[i].listenIP != keys[j].listenIP {
			return keys[i].listenIP < keys[j].listenIP
		}
		return keys[i].port < keys[j].port
	})
	var builder strings.Builder
	builder.WriteString("global\n")
	builder.WriteString("  master-worker\n")
	builder.WriteString("  stats socket ")
	builder.WriteString(paths.socketFile)
	builder.WriteString(" mode 600 level admin expose-fd listeners\n")
	builder.WriteString("  pidfile ")
	builder.WriteString(paths.pidFile)
	builder.WriteString("\n\n")
	builder.WriteString("defaults\n")
	builder.WriteString("  mode tcp\n")
	builder.WriteString("  timeout connect 5s\n")
	builder.WriteString("  timeout client 1m\n")
	builder.WriteString("  timeout server 1m\n\n")
	for _, key := range keys {
		rules := grouped[key]
		sort.Slice(rules, func(i int, j int) bool { return haproxyRuleKey(rules[i]) < haproxyRuleKey(rules[j]) })
		frontendName := "fe_" + sanitizeName(listenerKeyString(key))
		builder.WriteString("frontend ")
		builder.WriteString(frontendName)
		builder.WriteString("\n")
		builder.WriteString("  bind ")
		builder.WriteString(net.JoinHostPort(key.listenIP, strconv.Itoa(key.port)))
		if strings.TrimSpace(rules[0].ProxyProtocolIn) != "" && strings.TrimSpace(rules[0].ProxyProtocolIn) != "NONE" {
			builder.WriteString(" accept-proxy")
		}
		builder.WriteString("\n")
		if listenerHasTLSSNI(rules) {
			builder.WriteString("  tcp-request inspect-delay 5s\n")
			builder.WriteString("  tcp-request content accept if { req_ssl_hello_type 1 }\n")
		}
		for _, rule := range rules {
			backendName := haproxyBackendName(rule)
			if rule.MatchType == string(domain.MatchTypeAnyInbound) {
				builder.WriteString("  default_backend ")
				builder.WriteString(backendName)
				builder.WriteString("\n")
				continue
			}
			aclName := "sni_" + sanitizeName(haproxyRuleKey(rule))
			if !validHAProxySNIValue(rule.SNIHostname) {
				return "", configApplyError("invalid TLS_SNI hostname", unsupportedRuleError(rule, ModeHAProxy, "TLS_SNI hostname must be a hostname token"))
			}
			builder.WriteString("  acl ")
			builder.WriteString(aclName)
			builder.WriteString(" req.ssl_sni -i ")
			builder.WriteString(rule.SNIHostname)
			builder.WriteString("\n")
			builder.WriteString("  use_backend ")
			builder.WriteString(backendName)
			builder.WriteString(" if ")
			builder.WriteString(aclName)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
		for _, rule := range rules {
			renderHAProxyBackend(&builder, rule)
		}
	}
	return builder.String(), nil
}

func validHAProxySNIValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 253 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '-' || char == '.':
		default:
			return false
		}
	}
	return true
}

func renderHAProxyBackend(builder *strings.Builder, rule agent.RuleConfig) {
	builder.WriteString("backend ")
	builder.WriteString(haproxyBackendName(rule))
	builder.WriteString("\n")
	builder.WriteString("  balance source\n")
	targets := upstreamTargets(rule)
	if len(targets) == 0 {
		builder.WriteString("\n")
		return
	}
	minPriority := 0
	hasEnabledTarget := false
	for _, target := range targets {
		if !target.endpoint.Enabled {
			continue
		}
		if !hasEnabledTarget || target.priority < minPriority {
			minPriority = target.priority
		}
		hasEnabledTarget = true
	}
	if !hasEnabledTarget {
		builder.WriteString("\n")
		return
	}
	hasBackupTarget := false
	for _, target := range targets {
		if target.endpoint.Enabled && target.priority > minPriority {
			hasBackupTarget = true
			break
		}
	}
	for index, target := range targets {
		if !target.endpoint.Enabled {
			continue
		}
		builder.WriteString("  server ")
		builder.WriteString(sanitizeName(fmt.Sprintf("%s_%d", target.endpoint.ID, index)))
		builder.WriteString(" ")
		builder.WriteString(net.JoinHostPort(target.endpoint.Host, strconv.Itoa(target.endpoint.Port)))
		if sendIP := strings.TrimSpace(rule.SendIP); sendIP != "" {
			builder.WriteString(" source ")
			builder.WriteString(sendIP)
		}
		if target.priority > minPriority {
			builder.WriteString(" backup")
		}
		if hasBackupTarget {
			builder.WriteString(" check")
		}
		switch strings.TrimSpace(rule.ProxyProtocolOut) {
		case "V1", "PROXY_V1":
			builder.WriteString(" send-proxy")
		case "V2", "PROXY_V2":
			builder.WriteString(" send-proxy-v2")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func haproxyBackendName(rule agent.RuleConfig) string {
	return "be_" + sanitizeName(haproxyRuleKey(rule))
}

func haproxyRuleKey(rule agent.RuleConfig) string {
	if strings.TrimSpace(rule.RuntimeID) != "" {
		return strings.TrimSpace(rule.RuntimeID)
	}
	return strings.TrimSpace(rule.ID)
}

type prioritizedTarget struct {
	priority int
	endpoint agent.TargetEndpoint
}

func upstreamTargets(rule agent.RuleConfig) []prioritizedTarget {
	switch rule.Upstream.Type {
	case "TARGET":
		if rule.Upstream.Target == nil {
			return nil
		}
		return []prioritizedTarget{{priority: 10, endpoint: *rule.Upstream.Target}}
	case "TARGET_GROUP":
		targets := make([]prioritizedTarget, 0)
		for _, bucket := range rule.Upstream.TargetGroup {
			for _, target := range bucket.Targets {
				targets = append(targets, prioritizedTarget{priority: bucket.Priority, endpoint: target})
			}
		}
		sort.SliceStable(targets, func(i int, j int) bool {
			if targets[i].priority != targets[j].priority {
				return targets[i].priority < targets[j].priority
			}
			return targets[i].endpoint.ID < targets[j].endpoint.ID
		})
		return targets
	default:
		return nil
	}
}

func listenerHasTLSSNI(rules []agent.RuleConfig) bool {
	for _, rule := range rules {
		if rule.MatchType == string(domain.MatchTypeTLSSNI) {
			return true
		}
	}
	return false
}

func unsupportedBackendDetails(rules []agent.RuleConfig, dataplane string, message string) []agent.ConfigApplyErrorDetail {
	details := make([]agent.ConfigApplyErrorDetail, 0, len(rules))
	for _, rule := range rules {
		details = append(details, unsupportedRuleError(rule, dataplane, message))
	}
	return details
}

func haproxyValidateDetails(rules []agent.RuleConfig, message string) []agent.ConfigApplyErrorDetail {
	details := make([]agent.ConfigApplyErrorDetail, 0, len(rules))
	for _, rule := range rules {
		detail := unsupportedRuleError(rule, ModeHAProxy, message)
		detail.Code = ErrorHAProxyValidateFailed
		details = append(details, detail)
	}
	return details
}

func readPID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
