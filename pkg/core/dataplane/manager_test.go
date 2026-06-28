package dataplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestManagerReturnsStructuredUnsupportedRuleForForcedNFTablesSNI(t *testing.T) {
	manager := NewManager(Options{Mode: ModeAuto, InstanceID: "test"})
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{{
			ID:        "rule_1",
			Enabled:   true,
			Dataplane: ModeNFTables,
			Protocol:  domain.ProtocolTCP,
			ListenIP:  "0.0.0.0",
			Port:      443,
			MatchType: string(domain.MatchTypeTLSSNI),
			Upstream: agent.RuleUpstreamConfig{Type: "TARGET", Target: &agent.TargetEndpoint{
				ID:      "target_1",
				Host:    "1.1.1.1",
				Port:    443,
				Enabled: true,
			}},
		}},
	})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 {
		t.Fatalf("expected one structured error, got %#v from %v", details, err)
	}
	if details[0].Code != ErrorDataplaneUnsupportedRule || details[0].Dataplane != ModeNFTables {
		t.Fatalf("unexpected detail: %#v", details[0])
	}
}

func TestManagerDetectsMixedDataplaneListenerConflict(t *testing.T) {
	manager := NewManager(Options{Mode: ModeAuto, InstanceID: "test"})
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			anyRule("rule_native", ModeNative, domain.ProtocolTCP, 10000),
			anyRule("rule_haproxy", ModeHAProxy, domain.ProtocolTCP, 10000),
		},
	})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 {
		t.Fatalf("expected one conflict, got %#v from %v", details, err)
	}
	if details[0].Code != ErrorListenerConflict || details[0].Dataplane != "MIXED" {
		t.Fatalf("unexpected detail: %#v", details[0])
	}
}

func TestNFTablesTableNamePreservesLongInstanceUniqueness(t *testing.T) {
	first := strings.Repeat("a", 70) + "one"
	second := strings.Repeat("a", 70) + "two"
	firstName := nftablesTableNameForInstance(first)
	secondName := nftablesTableNameForInstance(second)
	if firstName == secondName {
		t.Fatalf("long instance ids must not collide after sanitization: %q", firstName)
	}
	if !strings.HasPrefix(firstName, "prism_") || !strings.HasPrefix(secondName, "prism_") {
		t.Fatalf("table names must keep prism prefix, got %q and %q", firstName, secondName)
	}
}

func TestRenderHAProxyConfigSupportsMultipleTLSRulesOnOneListener(t *testing.T) {
	config, err := renderHAProxyConfig(agent.ConfigSnapshot{
		ConfigVersion: 7,
		Rules: []agent.RuleConfig{
			tlsRule("rule_a", "a.example.com"),
			tlsRule("rule_b", "b.example.com"),
		},
	}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if err != nil {
		t.Fatalf("render haproxy config: %v", err)
	}
	for _, fragment := range []string{
		"frontend fe_TCP_0_0_0_0_443",
		"acl sni_rule_a req.ssl_sni -i a.example.com",
		"use_backend be_rule_b if sni_rule_b",
		"backend be_rule_a",
		"balance source",
	} {
		if !strings.Contains(config, fragment) {
			t.Fatalf("expected config to contain %q:\n%s", fragment, config)
		}
	}
}

func TestRenderNFTablesConfigRequiresLiteralIPTarget(t *testing.T) {
	_, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{{
			ID:        "rule_hostname",
			Protocol:  domain.ProtocolTCP,
			ListenIP:  "0.0.0.0",
			Port:      10000,
			MatchType: string(domain.MatchTypeAnyInbound),
			Upstream: agent.RuleUpstreamConfig{Type: "TARGET", Target: &agent.TargetEndpoint{
				ID:      "target_1",
				Host:    "example.com",
				Port:    443,
				Enabled: true,
			}},
		}},
	}, "prism_test", false)
	if len(agent.StructuredApplyErrors(err)) != 1 {
		t.Fatalf("expected structured literal IP error, got %v", err)
	}
}

func TestNFTablesCleanupPropagatesDestroyErrors(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{{output: []byte("Operation not permitted"), err: errors.New("exit status 1")}}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	err := backend.cleanupTable(context.Background(), nil)
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorNFTablesLocked {
		t.Fatalf("expected cleanup destroy error to propagate, got err=%v details=%#v", err, details)
	}
	if len(runner.calls) != 1 || strings.Join(runner.calls[0], " ") != "nft destroy table inet "+nftablesTableNameForInstance("test") {
		t.Fatalf("expected not-found-safe destroy without pre-list, calls=%#v", runner.calls)
	}
}

func TestNFTablesCleanupIgnoresMissingTable(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{{output: []byte("No such file or directory"), err: errors.New("exit status 1")}}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	if err := backend.cleanupTable(context.Background(), nil); err != nil {
		t.Fatalf("missing nftables table should be ignored: %v", err)
	}
}

func TestNFTablesCleanupPropagatesMissingBinary(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{{err: errors.New(`fork/exec /usr/sbin/nft: no such file or directory`)}}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	err := backend.cleanupTable(context.Background(), nil)
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorNFTablesLocked {
		t.Fatalf("missing nft binary must not be treated as a missing table, got err=%v details=%#v", err, details)
	}
}

func TestNFTablesApplyRejectsOccupiedTCPListener(t *testing.T) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("reserve tcp listener: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	rule := anyRule("rule_occupied", ModeNFTables, domain.ProtocolTCP, port)
	rule.ListenIP = "0.0.0.0"
	backend := NewNFTablesBackend(Options{
		InstanceID:    "test",
		StateDir:      t.TempDir(),
		CommandRunner: &recordingCommandRunner{},
	})
	err = backend.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByExternalProcess || details[0].Dataplane != ModeNFTables {
		t.Fatalf("expected occupied listener structured error, got err=%v details=%#v", err, details)
	}
}

func TestRenderNFTablesConfigOmitsDisabledTargetRule(t *testing.T) {
	rule := anyRule("rule_disabled", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.Upstream.Target.Enabled = false
	if !nftablesSupportsRule(rule) {
		t.Fatalf("disabled literal target rule must still be supported so stale nftables state can be removed")
	}
	config, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 3,
		ConfigHash:    "abc123",
		Rules:         []agent.RuleConfig{rule},
	}, "prism_test", true)
	if err != nil {
		t.Fatalf("render disabled target nftables config: %v", err)
	}
	for _, unexpected := range []string{"dnat", "masquerade", "rule_disabled"} {
		if strings.Contains(config, unexpected) {
			t.Fatalf("disabled target rule must not render active nftables NAT entry containing %q:\n%s", unexpected, config)
		}
	}
}

func TestRenderNFTablesConfigMatchesSpecificListenIPAndExpandsTCPUDP(t *testing.T) {
	rule := anyRule("rule_1", ModeNFTables, domain.ProtocolTCPUDP, 10000)
	rule.ListenIP = "203.0.113.10"
	config, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 3,
		ConfigHash:    "abc123",
		Rules:         []agent.RuleConfig{rule},
	}, "prism_test", true)
	if err != nil {
		t.Fatalf("render nftables config: %v", err)
	}
	for _, fragment := range []string{
		"delete table inet prism_test",
		"ip daddr 203.0.113.10 tcp dport 10000",
		"ip daddr 203.0.113.10 udp dport 10000",
		"type nat hook postrouting priority srcnat",
		"meta mark set 0x70726973 dnat ip to 1.1.1.1:443",
		"ip daddr 1.1.1.1 tcp dport 443 meta mark 0x70726973 masquerade",
		"ip daddr 1.1.1.1 udp dport 443 meta mark 0x70726973 masquerade",
	} {
		if !strings.Contains(config, fragment) {
			t.Fatalf("expected config to contain %q:\n%s", fragment, config)
		}
	}
	if strings.Contains(config, "tcp_udp") {
		t.Fatalf("nftables config must not emit tcp_udp token:\n%s", config)
	}
}

func TestRenderNFTablesConfigPreservesWildcardListenAddressFamily(t *testing.T) {
	ipv4Rule := anyRule("rule_v4_wildcard", ModeNFTables, domain.ProtocolTCP, 10000)
	ipv4Rule.ListenIP = "0.0.0.0"
	ipv4Rule.Upstream.Target.Host = "1.1.1.1"
	ipv4Rule.Upstream.Target.Port = 443
	ipv6Rule := anyRule("rule_v6_wildcard", ModeNFTables, domain.ProtocolTCP, 10001)
	ipv6Rule.ListenIP = "::"
	ipv6Rule.Upstream.Target.Host = "2001:db8::1"
	ipv6Rule.Upstream.Target.Port = 443
	config, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 3,
		ConfigHash:    "abc123",
		Rules:         []agent.RuleConfig{ipv4Rule, ipv6Rule},
	}, "prism_test", false)
	if err != nil {
		t.Fatalf("render nftables config: %v", err)
	}
	for _, fragment := range []string{
		"fib daddr type local ip daddr 0.0.0.0/0 tcp dport 10000",
		"fib daddr type local ip6 daddr ::/0 tcp dport 10001",
	} {
		if !strings.Contains(config, fragment) {
			t.Fatalf("expected wildcard family match %q:\n%s", fragment, config)
		}
	}
}

func TestRenderNFTablesConfigRejectsCrossFamilyDNAT(t *testing.T) {
	rule := anyRule("rule_cross_family", ModeNFTables, domain.ProtocolUDP, 10000)
	rule.ListenIP = "0.0.0.0"
	rule.Upstream.Target.Host = "2001:db8::1"
	_, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 3,
		ConfigHash:    "abc123",
		Rules:         []agent.RuleConfig{rule},
	}, "prism_test", false)
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorDataplaneUnsupportedRule {
		t.Fatalf("expected cross-family DNAT unsupported error, got err=%v details=%#v", err, details)
	}
	if nftablesSupportsRule(rule) {
		t.Fatalf("nftables support check must reject cross-family DNAT")
	}
}

func TestRenderNFTablesConfigBracketsIPv6TargetWithPort(t *testing.T) {
	rule := anyRule("rule_v6", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.ListenIP = "::"
	rule.Upstream.Target.Host = "2001:db8::1"
	rule.Upstream.Target.Port = 443
	config, err := renderNFTablesConfig(agent.ConfigSnapshot{
		ConfigVersion: 3,
		ConfigHash:    "abc123",
		Rules:         []agent.RuleConfig{rule},
	}, "prism_test", false)
	if err != nil {
		t.Fatalf("render nftables config: %v", err)
	}
	if !strings.Contains(config, "dnat ip6 to [2001:db8::1]:443") {
		t.Fatalf("expected bracketed IPv6 DNAT target:\n%s", config)
	}
}

func TestRenderHAProxyConfigUsesEnabledTargetsForPrimaryPriority(t *testing.T) {
	rule := anyRule("rule_group", ModeHAProxy, domain.ProtocolTCP, 10000)
	rule.Upstream = agent.RuleUpstreamConfig{
		Type: "TARGET_GROUP",
		TargetGroup: []agent.TargetPriorityBucket{
			{Priority: 1, Targets: []agent.TargetEndpoint{{ID: "disabled", Host: "1.1.1.1", Port: 443, Enabled: false}}},
			{Priority: 10, Targets: []agent.TargetEndpoint{{ID: "primary", Host: "1.1.1.2", Port: 443, Enabled: true}}},
			{Priority: 20, Targets: []agent.TargetEndpoint{{ID: "backup", Host: "1.1.1.3", Port: 443, Enabled: true}}},
		},
	}
	config, err := renderHAProxyConfig(agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if err != nil {
		t.Fatalf("render haproxy config: %v", err)
	}
	if strings.Contains(config, "primary_0 1.1.1.2:443 backup") {
		t.Fatalf("enabled primary bucket must not be marked backup when lower priority bucket is disabled:\n%s", config)
	}
	if !strings.Contains(config, "backup_2 1.1.1.3:443 backup") {
		t.Fatalf("higher priority enabled bucket should remain backup:\n%s", config)
	}
	if !strings.Contains(config, "primary_1 1.1.1.2:443 check") || !strings.Contains(config, "backup_2 1.1.1.3:443 backup check") {
		t.Fatalf("priority failover must enable HAProxy health checks on primary and backup servers:\n%s", config)
	}
}

func TestRenderHAProxyConfigRejectsUnsafeSNIHostname(t *testing.T) {
	_, err := renderHAProxyConfig(agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			tlsRule("rule_unsafe", "app.example.com\nuse_backend injected"),
		},
	}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if len(agent.StructuredApplyErrors(err)) != 1 {
		t.Fatalf("expected structured unsafe SNI error, got %v", err)
	}
}

func TestHAProxyApplyDoesNotRequestSocketTransferOnFirstStart(t *testing.T) {
	dir := t.TempDir()
	haproxyPath := filepath.Join(dir, "haproxy")
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write haproxy placeholder: %v", err)
	}
	runner := &recordingCommandRunner{}
	backend := NewHAProxyBackend(Options{
		HAProxyPath:   haproxyPath,
		StateDir:      filepath.Join(dir, "state"),
		RunDir:        filepath.Join(dir, "run"),
		InstanceID:    "test",
		CommandRunner: runner,
	})
	if err := backend.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)},
	}); err != nil {
		t.Fatalf("apply haproxy: %v", err)
	}
	if len(runner.calls) < 2 {
		t.Fatalf("expected validate and start calls, got %#v", runner.calls)
	}
	start := strings.Join(runner.calls[1], " ")
	if strings.Contains(start, " -x ") || strings.Contains(start, " -sf ") {
		t.Fatalf("first start must not request socket transfer or graceful stop, got %q", start)
	}
}

func TestHAProxyApplyIgnoresUnverifiedOldPIDForGracefulStop(t *testing.T) {
	dir := t.TempDir()
	haproxyPath := filepath.Join(dir, "haproxy")
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write haproxy placeholder: %v", err)
	}
	backend := NewHAProxyBackend(Options{
		HAProxyPath:   haproxyPath,
		StateDir:      filepath.Join(dir, "state"),
		RunDir:        filepath.Join(dir, "run"),
		InstanceID:    "test",
		CommandRunner: &recordingCommandRunner{},
	})
	paths := backend.paths()
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(paths.pidFile, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if err := os.WriteFile(paths.socketFile, []byte("socket"), 0o600); err != nil {
		t.Fatalf("write socket: %v", err)
	}
	runner := backend.options.CommandRunner.(*recordingCommandRunner)
	if err := backend.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)},
	}); err != nil {
		t.Fatalf("apply haproxy: %v", err)
	}
	start := strings.Join(runner.calls[1], " ")
	if strings.Contains(start, " -x ") || strings.Contains(start, " -sf ") {
		t.Fatalf("unverified old pid must not be passed to haproxy reload, got %q", start)
	}
}

func TestHAProxyApplyReportsStartupFailure(t *testing.T) {
	dir := t.TempDir()
	haproxyPath := filepath.Join(dir, "haproxy")
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write haproxy placeholder: %v", err)
	}
	runner := &recordingCommandRunner{outputs: []commandResult{
		{output: []byte("configuration file is valid\n")},
		{output: []byte("cannot bind socket\n"), err: errors.New("exit status 1")},
	}}
	backend := NewHAProxyBackend(Options{
		HAProxyPath:   haproxyPath,
		StateDir:      filepath.Join(dir, "state"),
		RunDir:        filepath.Join(dir, "run"),
		InstanceID:    "test",
		CommandRunner: runner,
	})
	err := backend.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)},
	})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || !strings.Contains(err.Error(), "cannot bind socket") {
		t.Fatalf("expected HAProxy startup failure to be reported, got %v %#v", err, details)
	}
}

func TestNFTablesCleanupFailureIsReported(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{
		{output: []byte("permission denied\n"), err: errors.New("exit status 1")},
	}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	err := backend.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 2})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorNFTablesLocked || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected nftables cleanup failure to be reported, got %v %#v", err, details)
	}
}

func TestManagerUsesInstallModeWhenControlPlaneModeIsAuto(t *testing.T) {
	manager := NewManager(Options{Mode: ModeNFTables, InstanceID: "test"})
	plan, err := manager.plan(agent.ConfigSnapshot{
		ConfigVersion: 1,
		DataplaneMode: ModeAuto,
		Rules:         []agent.RuleConfig{anyRule("rule_1", "", domain.ProtocolUDP, 10000)},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.rules[ModeNFTables]) != 1 {
		t.Fatalf("expected local install mode to select NFTABLES, got %#v", plan.rules)
	}
}

func TestManagerRollsBackExternalListenerLocksWhenManagedApplyFails(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:       ModeAuto,
		InstanceID: "instance_a",
		RunDir:     filepath.Join(dir, "run"),
		StateDir:   filepath.Join(dir, "state"),
		InstallDir: filepath.Join(dir, "missing-install"),
	})
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)},
	})
	if err == nil {
		t.Fatalf("expected missing HAProxy apply to fail")
	}
	lockPath := filepath.Join(dir, "run", "listeners", sanitizeName("TCP/0.0.0.0:10000")+".lock")
	if _, statErr := os.Stat(lockPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected failed apply to remove listener lock, stat err=%v", statErr)
	}
}

func TestManagerStopOutgoingHAProxyBeforeNativeTakesListener(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:       ModeAuto,
		InstanceID: "instance_a",
		RunDir:     filepath.Join(dir, "run"),
		StateDir:   filepath.Join(dir, "state"),
		InstallDir: filepath.Join(dir, "install"),
	})
	paths := manager.haproxy.paths()
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(paths.pidFile, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	if err := os.WriteFile(paths.socketFile, []byte("socket"), 0o600); err != nil {
		t.Fatalf("write socket file: %v", err)
	}
	current, err := manager.plan(agent.ConfigSnapshot{
		ConfigVersion: 2,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeNative, domain.ProtocolTCP, 10000)},
	})
	if err != nil {
		t.Fatalf("plan current: %v", err)
	}

	manager.rememberSnapshot(agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)},
	})
	previousSnapshot, ok := manager.lastSnapshot()
	if !ok {
		t.Fatalf("expected previous snapshot")
	}
	if err := manager.stopOutgoingExternalBackends(context.Background(), previousSnapshot, true, current, agent.ConfigSnapshot{ConfigVersion: 2}); err != nil {
		t.Fatalf("stop outgoing haproxy: %v", err)
	}
	for _, path := range []string{paths.pidFile, paths.socketFile} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected outgoing HAProxy apply to remove %s, stat err=%v", path, err)
		}
	}
}

func TestManagerStagesOutgoingHAProxyListenersWhenAddingNewHAProxyListener(t *testing.T) {
	previousSnapshot := agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			anyRule("rule_old_haproxy", ModeHAProxy, domain.ProtocolTCP, 443),
			anyRule("rule_keep_haproxy", ModeHAProxy, domain.ProtocolTCP, 9443),
		},
	}
	currentSnapshot := agent.ConfigSnapshot{
		ConfigVersion: 2,
		Rules: []agent.RuleConfig{
			anyRule("rule_native", ModeNative, domain.ProtocolTCP, 443),
			anyRule("rule_keep_haproxy", ModeHAProxy, domain.ProtocolTCP, 9443),
			anyRule("rule_new_haproxy", ModeHAProxy, domain.ProtocolTCP, 8443),
		},
	}
	manager := NewManager(Options{Mode: ModeAuto, InstanceID: "test"})
	previous, err := manager.plan(previousSnapshot)
	if err != nil {
		t.Fatalf("plan previous: %v", err)
	}
	current, err := manager.plan(currentSnapshot)
	if err != nil {
		t.Fatalf("plan current: %v", err)
	}

	retained := retainedExternalRules(previous, current, ModeHAProxy)
	if len(retained) != 1 || retained[0].ID != "rule_keep_haproxy" {
		t.Fatalf("expected staged HAProxy snapshot to retain only existing listeners, got %#v", retained)
	}
	if !hasOutgoingListener(previous, current, ModeHAProxy) {
		t.Fatalf("expected outgoing HAProxy listener to be detected")
	}
}

func TestManagerCleanupPartialNativeApplyWhenFirstExternalApplyFails(t *testing.T) {
	port := freeTCPPort(t)
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:       ModeAuto,
		InstanceID: "instance_a",
		RunDir:     filepath.Join(dir, "run"),
		StateDir:   filepath.Join(dir, "state"),
		InstallDir: filepath.Join(dir, "missing-install"),
	})
	native := anyRule("rule_native", ModeNative, domain.ProtocolTCP, port)
	native.ListenIP = "127.0.0.1"
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			native,
			anyRule("rule_haproxy", ModeHAProxy, domain.ProtocolTCP, port+1),
		},
	})
	if err == nil {
		t.Fatalf("expected missing HAProxy apply to fail")
	}
	listener, listenErr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	if listenErr != nil {
		t.Fatalf("expected failed first apply to clean up native listener on port %d: %v", port, listenErr)
	}
	_ = listener.Close()
}

func TestManagerRollbackStopsNativeListenersBeforeRestoringExternalBackends(t *testing.T) {
	port := freeTCPPort(t)
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:       ModeAuto,
		InstanceID: "instance_a",
		RunDir:     filepath.Join(dir, "run"),
		StateDir:   filepath.Join(dir, "state"),
		InstallDir: filepath.Join(dir, "missing-install"),
	})
	native := anyRule("rule_native", ModeNative, domain.ProtocolTCP, port)
	native.ListenIP = "127.0.0.1"
	if err := manager.native.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 2, Rules: []agent.RuleConfig{native}}); err != nil {
		t.Fatalf("apply native listener before rollback: %v", err)
	}

	manager.rollbackAfterFailedApply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{anyRule("rule_haproxy", ModeHAProxy, domain.ProtocolTCP, port)},
	}, true, plan{})

	listener, listenErr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	if listenErr != nil {
		t.Fatalf("expected rollback to stop native listener before external restore, port %d still busy: %v", port, listenErr)
	}
	_ = listener.Close()
}

func TestManagerRestoresPreviousLocksWhenStagedExternalShutdownFails(t *testing.T) {
	dir := t.TempDir()
	haproxyPath := filepath.Join(dir, "haproxy")
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write haproxy: %v", err)
	}
	runner := &recordingCommandRunner{outputs: []commandResult{{output: []byte("validation failed"), err: errors.New("exit status 1")}}}
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "instance_a",
		RunDir:        filepath.Join(dir, "run"),
		StateDir:      filepath.Join(dir, "state"),
		HAProxyPath:   haproxyPath,
		CommandRunner: runner,
	})
	previousRuleA := anyRule("rule_a", ModeHAProxy, domain.ProtocolTCP, freeTCPPort(t))
	previousRuleB := anyRule("rule_b", ModeHAProxy, domain.ProtocolTCP, freeTCPPort(t))
	previous := agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{previousRuleA, previousRuleB}}
	manager.rememberSnapshot(previous)
	currentRuleA := previousRuleA
	currentRuleA.Dataplane = ModeNative
	current := agent.ConfigSnapshot{ConfigVersion: 2, Rules: []agent.RuleConfig{currentRuleA, previousRuleB}}
	if err := manager.Apply(context.Background(), current); err == nil {
		t.Fatalf("expected staged HAProxy shutdown failure")
	}
	lockDir := filepath.Join(dir, "run", "listeners")
	for _, rule := range []agent.RuleConfig{previousRuleA, previousRuleB} {
		path := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(rule)))+".lock")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected previous lock restored for %s: %v", rule.ID, err)
		}
		if !strings.Contains(string(data), "dataplane=HAPROXY") {
			t.Fatalf("expected previous HAProxy lock restored, got:\n%s", data)
		}
	}
}

func TestManagerWriteListenerLockRejectsLiveOtherInstanceAtomically(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "listener.lock")
	rule := anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)
	key := listenerKeyForRule(rule)
	first := NewManager(Options{InstanceID: "instance_a", ServiceName: "agent-a"})
	second := NewManager(Options{InstanceID: "instance_b", ServiceName: "agent-b"})
	entries := []plannedRule{{rule: rule, dataplane: ModeHAProxy}}
	if err := first.writeListenerLock(lockPath, key, entries); err != nil {
		t.Fatalf("first lock acquisition: %v", err)
	}
	err := second.writeListenerLock(lockPath, key, entries)
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_a" {
		t.Fatalf("expected second instance to fail with owner detail, got err=%v details=%#v", err, details)
	}
}

func TestManagerReleasesEarlierListenerLocksWhenLaterLockFails(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{Mode: ModeAuto, InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	other := NewManager(Options{InstanceID: "instance_b", ServiceName: "agent-b"})
	ruleA := anyRule("rule_a", ModeHAProxy, domain.ProtocolTCP, 10000)
	ruleB := anyRule("rule_b", ModeHAProxy, domain.ProtocolTCP, 10001)
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	ruleBPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(ruleB)))+".lock")
	if err := other.writeListenerLock(ruleBPath, listenerKeyForRule(ruleB), []plannedRule{{rule: ruleB, dataplane: ModeHAProxy}}); err != nil {
		t.Fatalf("write competing lock: %v", err)
	}
	plan := plan{planned: []plannedRule{
		{rule: ruleA, dataplane: ModeHAProxy},
		{rule: ruleB, dataplane: ModeHAProxy},
	}}
	err := manager.ensureExternalListenerLocks(plan)
	if err == nil {
		t.Fatalf("expected later competing lock to fail")
	}
	ruleAPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(ruleA)))+".lock")
	if _, statErr := os.Stat(ruleAPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected earlier acquired lock to be released after later failure, stat err=%v", statErr)
	}
}

func TestManagerRejectsOverlappingExternalListenerLocks(t *testing.T) {
	dir := t.TempDir()
	first := NewManager(Options{Mode: ModeAuto, InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	second := NewManager(Options{Mode: ModeAuto, InstanceID: "instance_b", RunDir: filepath.Join(dir, "run")})
	wildcard := anyRule("rule_wildcard", ModeHAProxy, domain.ProtocolTCP, 10000)
	specific := anyRule("rule_specific", ModeHAProxy, domain.ProtocolTCP, 10000)
	specific.ListenIP = "127.0.0.1"
	if err := first.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: wildcard, dataplane: ModeHAProxy}}}); err != nil {
		t.Fatalf("lock wildcard listener: %v", err)
	}
	err := second.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: specific, dataplane: ModeHAProxy}}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_a" {
		t.Fatalf("expected overlapping listener lock to fail with owner detail, got err=%v details=%#v", err, details)
	}
}

func TestManagerLocksNativeListenersAgainstOtherInstances(t *testing.T) {
	dir := t.TempDir()
	first := NewManager(Options{Mode: ModeNative, InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	second := NewManager(Options{Mode: ModeAuto, InstanceID: "instance_b", RunDir: filepath.Join(dir, "run")})
	nativeRule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, 10000)
	nftRule := anyRule("rule_nft", ModeNFTables, domain.ProtocolTCP, 10000)
	if err := first.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: nativeRule, dataplane: ModeNative}}}); err != nil {
		t.Fatalf("native lock setup: %v", err)
	}
	err := second.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: nftRule, dataplane: ModeNFTables}}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_a" {
		t.Fatalf("expected nftables instance to be blocked by native listener lock, got err=%v details=%#v", err, details)
	}
}

func TestManagerPreservesStaleOtherInstanceNFTablesLockWhenTableExists(t *testing.T) {
	dir := t.TempDir()
	runner := &recordingCommandRunner{}
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "instance_b",
		RunDir:        filepath.Join(dir, "run"),
		CommandRunner: runner,
	})
	rule := anyRule("rule_nft", ModeNFTables, domain.ProtocolTCP, 10000)
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(rule)))+".lock")
	if err := os.WriteFile(lockPath, []byte("instance_id=instance_a\npid=999999\nprocess_start_time=stale\nservice=agent-a\ndataplane=NFTABLES\nlisten_protocol=TCP\nlisten_ip=0.0.0.0\nlisten_port=10000\n"), 0o600); err != nil {
		t.Fatalf("write stale nftables lock: %v", err)
	}

	err := manager.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: rule, dataplane: ModeNFTables}}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_a" {
		t.Fatalf("expected stale nftables lock with existing table to be preserved, got err=%v details=%#v", err, details)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf("stale nftables lock must remain while old table may exist: %v", statErr)
	}
	if len(runner.calls) != 1 || strings.Join(runner.calls[0], " ") != "nft list table inet "+nftablesTableNameForInstance("instance_a") {
		t.Fatalf("expected stale lock check to probe old nftables table, calls=%#v", runner.calls)
	}
}

func TestManagerReplacesStaleOtherInstanceNFTablesLockWhenTableIsMissing(t *testing.T) {
	dir := t.TempDir()
	runner := &recordingCommandRunner{outputs: []commandResult{{output: []byte("No such file or directory"), err: errors.New("exit status 1")}}}
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "instance_b",
		RunDir:        filepath.Join(dir, "run"),
		CommandRunner: runner,
	})
	rule := anyRule("rule_nft", ModeNFTables, domain.ProtocolTCP, 10000)
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(rule)))+".lock")
	if err := os.WriteFile(lockPath, []byte("instance_id=instance_a\npid=999999\nprocess_start_time=stale\nservice=agent-a\ndataplane=NFTABLES\nlisten_protocol=TCP\nlisten_ip=0.0.0.0\nlisten_port=10000\n"), 0o600); err != nil {
		t.Fatalf("write stale nftables lock: %v", err)
	}

	if err := manager.ensureExternalListenerLocks(plan{planned: []plannedRule{{rule: rule, dataplane: ModeNFTables}}}); err != nil {
		t.Fatalf("stale nftables lock with missing table should be replaceable: %v", err)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read replaced lock: %v", err)
	}
	if !strings.Contains(string(data), "instance_id=instance_b") {
		t.Fatalf("expected current instance to replace stale lock, got:\n%s", data)
	}
}

func TestListenerOverlapKeepsIPv4AndIPv6WildcardsDistinct(t *testing.T) {
	if listenerKeysOverlap(
		listenerKey{protocol: domain.ProtocolTCP, listenIP: "0.0.0.0", port: 10000},
		listenerKey{protocol: domain.ProtocolTCP, listenIP: "::", port: 10000},
	) {
		t.Fatalf("IPv4 and IPv6 wildcard listeners should not overlap")
	}
	if !listenerKeysOverlap(
		listenerKey{protocol: domain.ProtocolTCP, listenIP: "0.0.0.0", port: 10000},
		listenerKey{protocol: domain.ProtocolTCP, listenIP: "127.0.0.1", port: 10000},
	) {
		t.Fatalf("IPv4 wildcard should overlap IPv4 specific listener")
	}
}

func TestManagerTreatsReusedPIDWithDifferentStartTimeAsStaleLock(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{InstanceID: "instance_a", ServiceName: "agent-a"})
	rule := anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)
	key := listenerKeyForRule(rule)
	lockPath := filepath.Join(dir, "listener.lock")
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("instance_id=instance_b\npid=%d\nprocess_start_time=definitely-not-current\nservice=agent-b\ndataplane=HAPROXY\nlisten_protocol=%s\nlisten_ip=%s\nlisten_port=%d\n", os.Getpid(), key.protocol, key.listenIP, key.port)), 0o600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	if err := manager.writeListenerLock(lockPath, key, []plannedRule{{rule: rule, dataplane: ModeHAProxy}}); err != nil {
		t.Fatalf("stale reused-pid lock should be replaceable: %v", err)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read replaced lock: %v", err)
	}
	if !strings.Contains(string(data), "instance_id=instance_a") {
		t.Fatalf("expected stale lock to be replaced by current manager, got:\n%s", data)
	}
}

func TestManagerEnsureExternalLocksKeepsOutgoingStaleLocks(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, sanitizeName("TCP/0.0.0.0:10000")+".lock")
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("instance_id=instance_a\npid=%d\nservice=agent-a\ndataplane=HAPROXY\n", os.Getpid())), 0o600); err != nil {
		t.Fatalf("write outgoing lock: %v", err)
	}
	if err := manager.ensureExternalListenerLocks(plan{}); err != nil {
		t.Fatalf("ensure native-only locks: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("outgoing lock must stay until external backend stop succeeds: %v", err)
	}
	if err := manager.removeStaleExternalListenerLocks(plan{}); err != nil {
		t.Fatalf("remove stale locks: %v", err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale outgoing lock should be removed after explicit stale cleanup, stat err=%v", err)
	}
}

func TestManagerWriteListenerLockRejectsLiveSameInstanceDifferentPID(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "listener.lock")
	rule := anyRule("rule_1", ModeHAProxy, domain.ProtocolTCP, 10000)
	key := listenerKeyForRule(rule)
	manager := NewManager(Options{InstanceID: "instance_a", ServiceName: "agent-a"})
	entries := []plannedRule{{rule: rule, dataplane: ModeHAProxy}}
	process := exec.Command("sleep", "30")
	if err := process.Start(); err != nil {
		t.Skipf("start helper process: %v", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("instance_id=instance_a\npid=%d\nservice=agent-a\ndataplane=HAPROXY\n", process.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	err := manager.writeListenerLock(lockPath, key, entries)
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_a" {
		t.Fatalf("expected live same-instance lock with different pid to be rejected, got err=%v details=%#v", err, details)
	}
}

func TestManagerDoesNotRemoveLiveSameInstanceStaleListenerLocksFromOtherPID(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	process := exec.Command("sleep", "30")
	if err := process.Start(); err != nil {
		t.Skipf("start helper process: %v", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, sanitizeName("TCP/0.0.0.0:10000")+".lock")
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("instance_id=instance_a\npid=%d\nservice=agent-a\ndataplane=HAPROXY\n", process.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := manager.removeStaleListenerLocks(lockDir, nil); err != nil {
		t.Fatalf("remove stale locks: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("live same-instance lock from another pid must be preserved: %v", err)
	}
}

func TestNativeOnlyApplyChecksExistingNFTablesListenerLocks(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{Mode: ModeNative, InstanceID: "instance_a", RunDir: filepath.Join(dir, "run")})
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	lockPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(listenerKeyForRule(rule)))+".lock")
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("instance_id=instance_b\npid=%d\nservice=agent-b\ndataplane=NFTABLES\nlisten_protocol=TCP\nlisten_ip=%s\nlisten_port=%d\n", os.Getpid(), rule.ListenIP, rule.Port)), 0o600); err != nil {
		t.Fatalf("write nftables lock: %v", err)
	}
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerOwnedByOtherPrismInstance || details[0].Owner != "instance_b" {
		t.Fatalf("expected native-only apply to honor existing nftables lock, got err=%v details=%#v", err, details)
	}
}

func TestManagerDetectsTCPUDPListenerConflictWithTCP(t *testing.T) {
	manager := NewManager(Options{Mode: ModeAuto, InstanceID: "test"})
	tcpUDP := anyRule("rule_tcp_udp", ModeNFTables, domain.ProtocolTCPUDP, 10000)
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			tcpUDP,
			anyRule("rule_tcp", ModeHAProxy, domain.ProtocolTCP, 10000),
		},
	})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorListenerConflict || details[0].Protocol != domain.ProtocolTCP {
		t.Fatalf("expected TCP_UDP to conflict on expanded TCP listener, got err=%v details=%#v", err, details)
	}
}

func anyRule(id string, dataplane string, protocol domain.Protocol, port int) agent.RuleConfig {
	return agent.RuleConfig{
		ID:        id,
		Enabled:   true,
		Dataplane: dataplane,
		Protocol:  protocol,
		ListenIP:  "0.0.0.0",
		Port:      port,
		MatchType: string(domain.MatchTypeAnyInbound),
		Upstream: agent.RuleUpstreamConfig{Type: "TARGET", Target: &agent.TargetEndpoint{
			ID:      "target_" + id,
			Host:    "1.1.1.1",
			Port:    443,
			Enabled: true,
		}},
	}
}

func tlsRule(id string, hostname string) agent.RuleConfig {
	rule := anyRule(id, ModeHAProxy, domain.ProtocolTCP, 443)
	rule.MatchType = string(domain.MatchTypeTLSSNI)
	rule.SNIHostname = hostname
	return rule
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("close reserved tcp port: %v", err)
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

type commandResult struct {
	output []byte
	err    error
}

type recordingCommandRunner struct {
	outputs []commandResult
	calls   [][]string
}

func (runner *recordingCommandRunner) CombinedOutput(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, append([]string{name}, args...))
	if len(runner.outputs) == 0 {
		return nil, nil
	}
	result := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return result.output, result.err
}

func (runner *recordingCommandRunner) Start(context.Context, string, ...string) error {
	return nil
}
