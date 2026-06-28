package dataplane

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestManagerCleansOwnedExternalStateBeforeFirstMixedApply(t *testing.T) {
	runner := &recordingCommandRunner{}
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "test",
		CommandRunner: runner,
		RunDir:        filepath.Join(t.TempDir(), "run"),
		StateDir:      filepath.Join(t.TempDir(), "state"),
	})
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	nftRule := anyRule("rule_nft", ModeNFTables, domain.ProtocolUDP, freeTCPPort(t))
	nftRule.ListenIP = "0.0.0.0"
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule, nftRule}}); err != nil {
		t.Fatalf("apply first mixed native/managed snapshot: %v", err)
	}
	if len(runner.calls) < 1 || runner.calls[0][0] != "nft" || runner.calls[0][1] != "destroy" {
		t.Fatalf("expected nftables owned-state cleanup before first mixed apply, calls=%#v", runner.calls)
	}
}

func TestManagerDoesNotRequireNFTablesCleanupForFirstNativeHAProxyApply(t *testing.T) {
	dir := t.TempDir()
	haproxyPath := filepath.Join(dir, "haproxy")
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write haproxy placeholder: %v", err)
	}
	runner := &recordingCommandRunner{outputs: []commandResult{
		{err: errors.New(`fork/exec /usr/sbin/nft: no such file or directory`)},
		{output: []byte("configuration file is valid\n")},
		{},
	}}
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "test",
		CommandRunner: runner,
		HAProxyPath:   haproxyPath,
		RunDir:        filepath.Join(dir, "run"),
		StateDir:      filepath.Join(dir, "state"),
	})
	nativeRule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	nativeRule.ListenIP = "127.0.0.1"
	haproxyRule := anyRule("rule_haproxy", ModeHAProxy, domain.ProtocolTCP, freeTCPPort(t))
	haproxyRule.ListenIP = "127.0.0.1"
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{nativeRule, haproxyRule}}); err != nil {
		t.Fatalf("native+haproxy apply must not require optional nft cleanup: %v", err)
	}
	if len(runner.calls) < 3 || runner.calls[0][0] != "nft" || runner.calls[0][1] != "destroy" {
		t.Fatalf("expected best-effort nft cleanup before haproxy apply, calls=%#v", runner.calls)
	}
}

func TestManagerBestEffortCleansManagedStateForFirstNativeOnlyApply(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{{err: errors.New(`fork/exec /usr/sbin/nft: no such file or directory`)}}}
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	manager := NewManager(Options{
		Mode:          ModeNative,
		InstanceID:    "test",
		CommandRunner: runner,
		RunDir:        filepath.Join(t.TempDir(), "run"),
		StateDir:      filepath.Join(t.TempDir(), "state"),
	})
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}}); err != nil {
		t.Fatalf("native-only apply must not fail optional managed cleanup: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "nft" || runner.calls[0][1] != "destroy" {
		t.Fatalf("native-only first apply should best-effort clean stale managed state, calls=%#v", runner.calls)
	}
}

func TestManagerSurfacesStaleHAProxyCleanupFailureOnFirstApply(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:       ModeNative,
		InstanceID: "test",
		RunDir:     filepath.Join(dir, "run"),
		StateDir:   filepath.Join(dir, "state"),
	})
	paths := manager.haproxy.paths()
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		t.Fatalf("mkdir haproxy run dir: %v", err)
	}
	if err := os.WriteFile(paths.pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("write haproxy pid: %v", err)
	}
	originalSignal := haproxySignalProcess
	originalLooksManaged := haproxyProcessLooksManaged
	haproxyProcessLooksManaged = func(*HAProxyBackend, int) bool { return true }
	haproxySignalProcess = func(*os.Process, os.Signal) error { return syscall.EPERM }
	defer func() {
		haproxySignalProcess = originalSignal
		haproxyProcessLooksManaged = originalLooksManaged
	}()
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}})
	if err == nil || !strings.Contains(err.Error(), "stop managed HAProxy") {
		t.Fatalf("expected stale HAProxy cleanup failure to fail first apply, got %v", err)
	}
}

func TestManagerChecksLiveOwnedLocksBeforeFirstCleanup(t *testing.T) {
	dir := t.TempDir()
	runner := &recordingCommandRunner{}
	manager := NewManager(Options{
		Mode:          ModeNative,
		InstanceID:    "test",
		CommandRunner: runner,
		RunDir:        filepath.Join(dir, "run"),
		StateDir:      filepath.Join(dir, "state"),
	})
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatalf("start placeholder process: %v", err)
	}
	defer func() {
		_ = other.Process.Kill()
		_, _ = other.Process.Wait()
	}()
	lockDir := filepath.Join(dir, "run", "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("make lock dir: %v", err)
	}
	key := listenerKeyForRule(rule)
	lockPath := filepath.Join(lockDir, sanitizeName(listenerKeyString(key))+".lock")
	content := "instance_id=test\npid=" + strconv.Itoa(other.Process.Pid) + "\nprocess_start_time=\nservice=other\ndataplane=HAPROXY\nlisten_protocol=TCP\nlisten_ip=127.0.0.1\nlisten_port=" + strconv.Itoa(rule.Port) + "\n"
	if err := os.WriteFile(lockPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}})
	if err == nil {
		t.Fatalf("expected live same-instance lock to block first cleanup")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("startup cleanup must not run before live lock check, calls=%#v", runner.calls)
	}
}

func TestManagerRemovesOwnedNativeLockAfterLeavingManagedMode(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(Options{
		Mode:          ModeAuto,
		InstanceID:    "test",
		CommandRunner: &recordingCommandRunner{},
		RunDir:        filepath.Join(dir, "run"),
		StateDir:      filepath.Join(dir, "state"),
	})
	nativeRule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	nativeRule.ListenIP = "127.0.0.1"
	nftRule := anyRule("rule_nft", ModeNFTables, domain.ProtocolUDP, freeTCPPort(t))
	nftRule.ListenIP = "0.0.0.0"
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{nativeRule, nftRule}}); err != nil {
		t.Fatalf("apply mixed native/nft snapshot: %v", err)
	}
	lockPath := filepath.Join(dir, "run", "listeners", sanitizeName(listenerKeyString(listenerKeyForRule(nativeRule)))+".lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected mixed apply to create native listener lock: %v", err)
	}
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 2, Rules: []agent.RuleConfig{nativeRule}}); err != nil {
		t.Fatalf("apply native-only transition: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("native lock should remain while native listener is still active: %v", err)
	}
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 3}); err != nil {
		t.Fatalf("apply empty native-only snapshot: %v", err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("native lock should be removed after leaving managed mode and removing rule, stat err=%v", err)
	}
}

func TestManagerDoesNotRequireRunLocksForNativeOnlyApply(t *testing.T) {
	dir := t.TempDir()
	blockedRunDir := filepath.Join(dir, "run-file")
	if err := os.WriteFile(blockedRunDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked run path: %v", err)
	}
	rule := anyRule("rule_native", ModeNative, domain.ProtocolTCP, freeTCPPort(t))
	rule.ListenIP = "127.0.0.1"
	manager := NewManager(Options{
		Mode:       ModeNative,
		InstanceID: "test",
		RunDir:     blockedRunDir,
		StateDir:   filepath.Join(dir, "state"),
	})
	if err := manager.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}}); err != nil {
		t.Fatalf("native-only apply must not require listener lock directory: %v", err)
	}
}
