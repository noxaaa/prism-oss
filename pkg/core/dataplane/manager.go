package dataplane

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/forward"
)

type Manager struct {
	options  Options
	native   *NativeBackend
	haproxy  *HAProxyBackend
	nft      *NFTablesBackend
	mu       sync.Mutex
	last     agent.ConfigSnapshot
	hasLast  bool
	resolved map[string]string
}

func NewManager(options Options) *Manager {
	options.Mode = normalizeMode(options.Mode, ModeNative)
	options.ConflictPolicy = normalizeConflictPolicy(options.ConflictPolicy)
	if strings.TrimSpace(options.ServiceName) == "" {
		options.ServiceName = "prism-node-agent"
	}
	if strings.TrimSpace(options.InstallDir) == "" {
		options.InstallDir = filepath.Join("/opt", options.ServiceName)
	}
	if strings.TrimSpace(options.StateDir) == "" {
		options.StateDir = filepath.Join("/var/lib", options.ServiceName, "dataplane")
	}
	if strings.TrimSpace(options.RunDir) == "" {
		options.RunDir = filepath.Join("/run", "prism")
	}
	if strings.TrimSpace(options.InstanceID) == "" {
		options.InstanceID = options.ServiceName
	}
	if options.CommandRunner == nil {
		options.CommandRunner = execCommandRunner{}
	}
	native := &NativeBackend{supervisor: forward.NewSupervisor()}
	return &Manager{
		options: options,
		native:  native,
		haproxy: NewHAProxyBackend(options),
		nft:     NewNFTablesBackend(options),
	}
}

func (manager *Manager) Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error {
	previous, hasPrevious := manager.lastSnapshot()
	plan, err := manager.plan(snapshot)
	if err != nil {
		return err
	}
	if !hasPrevious {
		if err := manager.ensureNoLiveOwnedListenerLocksFromOtherProcess(plan); err != nil {
			return err
		}
		if err := manager.cleanupOwnedExternalBackends(ctx, plan); err != nil {
			return err
		}
	}
	if plan.hasManaged() || manager.listenerLockDirExists() {
		if err := manager.ensureExternalListenerLocks(plan); err != nil {
			return err
		}
	}
	if err := manager.stopOutgoingExternalBackends(ctx, previous, hasPrevious, plan, snapshot); err != nil {
		manager.restorePreviousListenerLocks(previous, hasPrevious, plan)
		return err
	}
	if plan.hasManaged() || manager.snapshotHasManaged(previous, hasPrevious) || manager.listenerLockDirExists() {
		if err := manager.removeStaleExternalListenerLocks(plan); err != nil {
			manager.releaseExternalListenerLocks(plan)
			return err
		}
	}
	if err := manager.native.Apply(ctx, plan.snapshotFor(ModeNative, snapshot)); err != nil {
		manager.releaseExternalListenerLocks(plan)
		manager.rollbackAfterFailedApply(ctx, previous, hasPrevious, plan)
		return annotateBackendErrors(err, ModeNative)
	}
	if len(plan.rules[ModeHAProxy]) > 0 || manager.snapshotHasDataplane(previous, hasPrevious, ModeHAProxy) {
		if err := manager.haproxy.Apply(ctx, plan.snapshotFor(ModeHAProxy, snapshot)); err != nil {
			manager.releaseExternalListenerLocks(plan)
			manager.rollbackAfterFailedApply(ctx, previous, hasPrevious, plan)
			return annotateBackendErrors(err, ModeHAProxy)
		}
	}
	if len(plan.rules[ModeNFTables]) > 0 || manager.snapshotHasDataplane(previous, hasPrevious, ModeNFTables) {
		if err := manager.nft.Apply(ctx, plan.snapshotFor(ModeNFTables, snapshot)); err != nil {
			manager.releaseExternalListenerLocks(plan)
			manager.rollbackAfterFailedApply(ctx, previous, hasPrevious, plan)
			return annotateBackendErrors(err, ModeNFTables)
		}
	}
	manager.rememberSnapshot(snapshot)
	manager.rememberResolvedPlan(plan)
	return nil
}

func (manager *Manager) AgentMetrics() agent.MetricsPayload {
	return mergeMetricsPayloads(manager.native.AgentMetrics(), manager.haproxy.AgentMetrics(), manager.nft.AgentMetrics())
}

func (manager *Manager) Close() error {
	var first error
	for _, backend := range []Backend{manager.nft, manager.haproxy, manager.native} {
		if err := backend.CleanupOwnedState(context.Background()); err != nil && first == nil {
			first = err
		}
		if err := backend.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (manager *Manager) CleanupOwnedState(ctx context.Context) error {
	var first error
	for _, backend := range []Backend{manager.nft, manager.haproxy, manager.native} {
		if err := backend.CleanupOwnedState(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (manager *Manager) lastSnapshot() (agent.ConfigSnapshot, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.last, manager.hasLast
}

func (manager *Manager) rememberSnapshot(snapshot agent.ConfigSnapshot) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.last = snapshot
	manager.hasLast = true
}

func (manager *Manager) rollbackAfterFailedApply(ctx context.Context, previous agent.ConfigSnapshot, ok bool, current plan) {
	if !ok {
		if current.hasManaged() {
			_ = manager.CleanupOwnedState(ctx)
		}
		return
	}
	rollbackPlan, err := manager.plan(previous)
	if err != nil {
		return
	}
	_ = manager.native.Apply(ctx, agent.ConfigSnapshot{})
	_ = manager.haproxy.Apply(ctx, rollbackPlan.snapshotFor(ModeHAProxy, previous))
	_ = manager.nft.Apply(ctx, rollbackPlan.snapshotFor(ModeNFTables, previous))
	_ = manager.native.Apply(ctx, rollbackPlan.snapshotFor(ModeNative, previous))
	_ = manager.ensureExternalListenerLocks(rollbackPlan)
}

func (manager *Manager) cleanupOwnedExternalBackends(ctx context.Context, plan plan) error {
	if len(plan.rules[ModeHAProxy]) > 0 {
		if err := manager.haproxy.Apply(ctx, agent.ConfigSnapshot{}); err != nil {
			return annotateBackendErrors(err, ModeHAProxy)
		}
	} else {
		if err := manager.haproxy.Apply(ctx, agent.ConfigSnapshot{}); err != nil {
			return annotateBackendErrors(err, ModeHAProxy)
		}
	}
	if len(plan.rules[ModeNFTables]) > 0 {
		if err := manager.nft.Apply(ctx, agent.ConfigSnapshot{}); err != nil {
			return annotateBackendErrors(err, ModeNFTables)
		}
	} else {
		_ = manager.nft.Apply(ctx, agent.ConfigSnapshot{})
	}
	return nil
}

func (manager *Manager) listenerLockDirExists() bool {
	info, err := os.Stat(filepath.Join(manager.options.RunDir, "listeners"))
	return err == nil && info.IsDir()
}

func (manager *Manager) snapshotHasManaged(snapshot agent.ConfigSnapshot, ok bool) bool {
	return manager.snapshotHasDataplane(snapshot, ok, ModeHAProxy) || manager.snapshotHasDataplane(snapshot, ok, ModeNFTables)
}

func (manager *Manager) snapshotHasDataplane(snapshot agent.ConfigSnapshot, ok bool, dataplane string) bool {
	if !ok {
		return false
	}
	plan, err := manager.plan(snapshot)
	if err != nil {
		return false
	}
	return len(plan.rules[dataplane]) > 0
}

func (manager *Manager) restorePreviousListenerLocks(previous agent.ConfigSnapshot, ok bool, current plan) {
	manager.releaseExternalListenerLocks(current)
	if !ok {
		return
	}
	previousPlan, err := manager.plan(previous)
	if err != nil {
		return
	}
	_ = manager.ensureExternalListenerLocks(previousPlan)
}

func (manager *Manager) stopOutgoingExternalBackends(ctx context.Context, previous agent.ConfigSnapshot, ok bool, current plan, base agent.ConfigSnapshot) error {
	if !ok {
		return nil
	}
	previousPlan, err := manager.plan(previous)
	if err != nil {
		return nil
	}
	if hasOutgoingListener(previousPlan, current, ModeHAProxy) {
		staged := base
		staged.Rules = retainedExternalRules(previousPlan, current, ModeHAProxy)
		if err := manager.haproxy.Apply(ctx, staged); err != nil {
			return annotateBackendErrors(err, ModeHAProxy)
		}
	}
	if hasOutgoingListener(previousPlan, current, ModeNFTables) {
		staged := base
		staged.Rules = retainedExternalRules(previousPlan, current, ModeNFTables)
		if err := manager.nft.Apply(ctx, staged); err != nil {
			return annotateBackendErrors(err, ModeNFTables)
		}
	}
	return nil
}

func hasOutgoingListener(previous plan, current plan, dataplane string) bool {
	currentKeys := listenerKeySet(current.rules[dataplane])
	for key := range listenerKeySet(previous.rules[dataplane]) {
		if _, ok := currentKeys[key]; !ok {
			return true
		}
	}
	return false
}

func listenerKeySet(rules []agent.RuleConfig) map[listenerKey]struct{} {
	keys := make(map[listenerKey]struct{}, len(rules))
	for _, rule := range rules {
		for _, key := range listenerKeysForRule(rule) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func retainedExternalRules(previous plan, current plan, dataplane string) []agent.RuleConfig {
	previousKeys := listenerKeySet(previous.rules[dataplane])
	retained := make([]agent.RuleConfig, 0, len(current.rules[dataplane]))
	for _, rule := range current.rules[dataplane] {
		for _, key := range listenerKeysForRule(rule) {
			if _, ok := previousKeys[key]; ok {
				retained = append(retained, rule)
				break
			}
		}
	}
	return retained
}

func (manager *Manager) rememberResolvedPlan(plan plan) {
	resolved := make(map[string]string, len(plan.planned))
	for _, entry := range plan.planned {
		resolved[entry.rule.ID] = entry.dataplane
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.resolved = resolved
}

func (manager *Manager) ResolvedDataplanes() map[string]string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	copied := make(map[string]string, len(manager.resolved))
	for ruleID, dataplane := range manager.resolved {
		copied[ruleID] = dataplane
	}
	return copied
}

type plan struct {
	rules   map[string][]agent.RuleConfig
	planned []plannedRule
}

func (plan plan) hasManaged() bool {
	return len(plan.rules[ModeHAProxy]) > 0 || len(plan.rules[ModeNFTables]) > 0
}

func (plan plan) snapshotFor(dataplane string, base agent.ConfigSnapshot) agent.ConfigSnapshot {
	next := base
	next.Rules = append([]agent.RuleConfig(nil), plan.rules[dataplane]...)
	return next
}

func (manager *Manager) plan(snapshot agent.ConfigSnapshot) (plan, error) {
	nodeMode := normalizeMode(snapshot.DataplaneMode, ModeAuto)
	if nodeMode == ModeAuto {
		nodeMode = normalizeMode(manager.options.Mode, ModeAuto)
	}
	result := plan{rules: map[string][]agent.RuleConfig{
		ModeNative:   {},
		ModeHAProxy:  {},
		ModeNFTables: {},
	}}
	planned := make([]plannedRule, 0, len(snapshot.Rules))
	var details []agent.ConfigApplyErrorDetail
	for _, rule := range snapshot.Rules {
		dataplane, errDetail := manager.selectDataplane(nodeMode, rule)
		if errDetail.Code != "" {
			details = append(details, errDetail)
			continue
		}
		if !manager.backendSupportsRule(dataplane, rule) {
			details = append(details, unsupportedRuleError(rule, dataplane, fmt.Sprintf("%s does not support this rule shape", dataplane)))
			continue
		}
		result.rules[dataplane] = append(result.rules[dataplane], rule)
		planned = append(planned, plannedRule{rule: rule, dataplane: dataplane})
	}
	if conflictDetails := detectListenerConflicts(planned); len(conflictDetails) > 0 {
		details = append(details, conflictDetails...)
	}
	if len(details) > 0 {
		return plan{}, configApplyError("dataplane plan failed", details...)
	}
	result.planned = planned
	return result, nil
}

func (manager *Manager) selectDataplane(nodeMode string, rule agent.RuleConfig) (string, agent.ConfigApplyErrorDetail) {
	preference := normalizePreference(rule.Dataplane)
	if preference != ModeAuto {
		if nodeMode != ModeAuto && nodeMode != preference {
			return "", unsupportedRuleError(rule, preference, fmt.Sprintf("node dataplane mode %s does not allow rule preference %s", nodeMode, preference))
		}
		return preference, agent.ConfigApplyErrorDetail{}
	}
	if nodeMode != ModeAuto {
		return nodeMode, agent.ConfigApplyErrorDetail{}
	}
	return manager.autoSelectDataplane(rule)
}

func (manager *Manager) autoSelectDataplane(rule agent.RuleConfig) (string, agent.ConfigApplyErrorDetail) {
	if rule.Protocol == domain.ProtocolUDP && rule.MatchType == string(domain.MatchTypeAnyInbound) {
		if manager.backendSupportsRule(ModeNFTables, rule) {
			return ModeNFTables, agent.ConfigApplyErrorDetail{}
		}
		return ModeNative, agent.ConfigApplyErrorDetail{}
	}
	if rule.Protocol == domain.ProtocolTCP {
		if manager.backendSupportsRule(ModeHAProxy, rule) {
			return ModeHAProxy, agent.ConfigApplyErrorDetail{}
		}
		return "", unsupportedRuleError(rule, ModeHAProxy, "AUTO requires HAProxy for TCP TLS_SNI/ANY_INBOUND, but the rule is not HAProxy-compatible")
	}
	return ModeNative, agent.ConfigApplyErrorDetail{}
}

func (manager *Manager) backendSupportsRule(dataplane string, rule agent.RuleConfig) bool {
	switch dataplane {
	case ModeNative:
		return true
	case ModeHAProxy:
		return haproxySupportsRule(rule)
	case ModeNFTables:
		return nftablesSupportsRule(rule)
	default:
		return false
	}
}

func detectListenerConflicts(rules []plannedRule) []agent.ConfigApplyErrorDetail {
	grouped := map[listenerKey][]plannedRule{}
	for _, rule := range rules {
		for _, key := range listenerKeysForRule(rule.rule) {
			grouped[key] = append(grouped[key], rule)
		}
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
	var details []agent.ConfigApplyErrorDetail
	for leftIndex := 0; leftIndex < len(keys); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(keys); rightIndex++ {
			if !listenerKeysOverlap(keys[leftIndex], keys[rightIndex]) {
				continue
			}
			combined := append([]plannedRule{}, grouped[keys[leftIndex]]...)
			combined = append(combined, grouped[keys[rightIndex]]...)
			if detail, ok := listenerConflictDetail(keys[leftIndex], combined); ok {
				details = append(details, detail)
			}
		}
	}
	for _, key := range keys {
		if detail, ok := listenerConflictDetail(key, grouped[key]); ok {
			details = append(details, detail)
		}
	}
	return details
}

func listenerConflictDetail(key listenerKey, entries []plannedRule) (agent.ConfigApplyErrorDetail, bool) {
	if len(entries) <= 1 {
		return agent.ConfigApplyErrorDetail{}, false
	}
	dataplane := entries[0].dataplane
	hasAny := false
	sniByValue := map[string]string{}
	rules := make([]agent.RuleConfig, 0, len(entries))
	for _, entry := range entries {
		rules = append(rules, entry.rule)
		if entry.dataplane != dataplane {
			return listenerError(ErrorListenerConflict, key, rules, "MIXED", "", "one listener cannot be owned by multiple dataplanes"), true
		}
		if entry.rule.MatchType == string(domain.MatchTypeAnyInbound) {
			hasAny = true
		}
		if entry.rule.MatchType == string(domain.MatchTypeTLSSNI) {
			sni := strings.ToLower(strings.TrimSpace(entry.rule.SNIHostname))
			if previousRuleID, ok := sniByValue[sni]; ok && previousRuleID != entry.rule.ID {
				return listenerError(ErrorListenerConflict, key, rules, dataplane, "", "multiple TLS_SNI rules use the same listener and SNI hostname"), true
			}
			sniByValue[sni] = entry.rule.ID
		}
	}
	if hasAny {
		return listenerError(ErrorListenerConflict, key, rules, dataplane, "", "ANY_INBOUND listener cannot share protocol/listen IP/port with another rule"), true
	}
	if !sameProxyProtocol(entries) {
		return listenerError(ErrorListenerConflict, key, rules, dataplane, "", "rules sharing one listener must use the same incoming proxy protocol setting"), true
	}
	return agent.ConfigApplyErrorDetail{}, false
}

func sameProxyProtocol(entries []plannedRule) bool {
	if len(entries) == 0 {
		return true
	}
	value := strings.TrimSpace(entries[0].rule.ProxyProtocolIn)
	for _, entry := range entries[1:] {
		if strings.TrimSpace(entry.rule.ProxyProtocolIn) != value {
			return false
		}
	}
	return true
}

func listenerKeysOverlap(left listenerKey, right listenerKey) bool {
	return left.protocol == right.protocol && left.port == right.port && listenIPsOverlap(left.listenIP, right.listenIP)
}

func listenIPsOverlap(left string, right string) bool {
	left = normalizeListenIP(left)
	right = normalizeListenIP(right)
	if left == right {
		return true
	}
	leftIP := net.ParseIP(strings.Trim(left, "[]"))
	rightIP := net.ParseIP(strings.Trim(right, "[]"))
	if leftIP == nil || rightIP == nil {
		return false
	}
	if listenIPFamily(leftIP) != listenIPFamily(rightIP) {
		return false
	}
	return leftIP.IsUnspecified() || rightIP.IsUnspecified()
}

func listenIPFamily(ip net.IP) string {
	if ip.To4() == nil {
		return "ip6"
	}
	return "ip"
}

func annotateBackendErrors(err error, dataplane string) error {
	if err == nil {
		return nil
	}
	details := agent.StructuredApplyErrors(err)
	if len(details) == 0 {
		return err
	}
	for index := range details {
		if details[index].Dataplane == "" {
			details[index].Dataplane = dataplane
		}
	}
	return configApplyError(err.Error(), details...)
}
