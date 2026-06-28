package dataplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestHAProxyStopRemovesPidAndSocketFiles(t *testing.T) {
	dir := t.TempDir()
	backend := NewHAProxyBackend(Options{RunDir: filepath.Join(dir, "run"), StateDir: filepath.Join(dir, "state"), InstanceID: "test"})
	paths := backend.paths()
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(paths.pidFile, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if err := os.WriteFile(paths.socketFile, []byte("socket"), 0o600); err != nil {
		t.Fatalf("write socket: %v", err)
	}
	if err := backend.stopOwnedProcess(); err != nil {
		t.Fatalf("stop haproxy: %v", err)
	}
	for _, path := range []string{paths.pidFile, paths.socketFile} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
}

func TestHAProxyStopDoesNotSignalUnverifiedPID(t *testing.T) {
	dir := t.TempDir()
	backend := NewHAProxyBackend(Options{
		RunDir:      filepath.Join(dir, "run"),
		StateDir:    filepath.Join(dir, "state"),
		InstallDir:  filepath.Join(dir, "install"),
		HAProxyPath: filepath.Join(dir, "managed-haproxy"),
		InstanceID:  "test",
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
	if err := backend.stopOwnedProcess(); err != nil {
		t.Fatalf("stop haproxy: %v", err)
	}
	for _, path := range []string{paths.pidFile, paths.socketFile} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected stale %s to be removed without signaling current process, stat err=%v", path, err)
		}
	}
}

func TestHAProxyStopReturnsSignalErrorsForManagedProcess(t *testing.T) {
	dir := t.TempDir()
	installDir := filepath.Join(dir, "install")
	haproxyPath := filepath.Join(installDir, "current", "dataplane", "haproxy", "haproxy")
	if err := os.MkdirAll(filepath.Dir(haproxyPath), 0o755); err != nil {
		t.Fatalf("mkdir haproxy dir: %v", err)
	}
	if err := os.WriteFile(haproxyPath, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write haproxy script: %v", err)
	}
	process := exec.Command(haproxyPath)
	if err := process.Start(); err != nil {
		t.Skipf("start managed haproxy script: %v", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	backend := NewHAProxyBackend(Options{
		RunDir:      filepath.Join(dir, "run"),
		StateDir:    filepath.Join(dir, "state"),
		InstallDir:  installDir,
		HAProxyPath: haproxyPath,
		InstanceID:  "test",
	})
	paths := backend.paths()
	if err := os.MkdirAll(paths.runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(paths.pidFile, []byte(fmt.Sprint(process.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if err := os.WriteFile(paths.socketFile, []byte("socket"), 0o600); err != nil {
		t.Fatalf("write socket: %v", err)
	}
	originalSignal := haproxySignalProcess
	originalLooksManaged := haproxyProcessLooksManaged
	haproxyProcessLooksManaged = func(*HAProxyBackend, int) bool { return true }
	haproxySignalProcess = func(*os.Process, os.Signal) error { return syscall.EPERM }
	defer func() {
		haproxySignalProcess = originalSignal
		haproxyProcessLooksManaged = originalLooksManaged
	}()
	if err := backend.stopOwnedProcess(); err == nil {
		t.Fatalf("expected signal error to be returned")
	}
	for _, path := range []string{paths.pidFile, paths.socketFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("signal failure must keep runtime file %s, stat err=%v", path, err)
		}
	}
}

func TestHAProxyManagedExecutableMatchesWrapperBinary(t *testing.T) {
	dir := t.TempDir()
	wrapperPath := filepath.Join(dir, "haproxy")
	binaryPath := filepath.Join(dir, "haproxy.bin")
	if err := os.WriteFile(wrapperPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	backend := NewHAProxyBackend(Options{HAProxyPath: wrapperPath, InstanceID: "test"})
	if !backend.pathIsManagedHAProxyExecutable(binaryPath) {
		t.Fatalf("expected wrapper sibling haproxy.bin to be treated as managed HAProxy executable")
	}
}

func TestHAProxyManagedExecutableMatchesPreviousReleaseBinary(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "prism-node-agent")
	currentWrapper := filepath.Join(installDir, "current", "dataplane", "haproxy", "haproxy")
	previousBinary := filepath.Join(installDir, "releases", "v0.1.99", "dataplane", "haproxy", "haproxy.bin")
	if err := os.MkdirAll(filepath.Dir(currentWrapper), 0o755); err != nil {
		t.Fatalf("mkdir current wrapper: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(previousBinary), 0o755); err != nil {
		t.Fatalf("mkdir previous binary: %v", err)
	}
	if err := os.WriteFile(currentWrapper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write current wrapper: %v", err)
	}
	if err := os.WriteFile(previousBinary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write previous binary: %v", err)
	}
	backend := NewHAProxyBackend(Options{InstallDir: installDir, HAProxyPath: currentWrapper, InstanceID: "test"})
	if !backend.pathIsManagedHAProxyExecutable(previousBinary) {
		t.Fatalf("expected previous release haproxy.bin under install dir to be treated as managed")
	}
}

func TestHAProxyAgentMetricsPreservesLastCountersWhenScrapeFails(t *testing.T) {
	backend := NewHAProxyBackend(Options{InstanceID: "test", RunDir: filepath.Join(t.TempDir(), "run")})
	backend.last = map[string]managedTrafficSnapshot{"rule-1": {UploadBytes: 128}}
	payload := backend.AgentMetrics()
	if len(payload.TrafficDeltas) != 0 {
		t.Fatalf("failed HAProxy scrape must not emit deltas, got %#v", payload.TrafficDeltas)
	}
	if backend.last["rule-1"].UploadBytes != 128 {
		t.Fatalf("failed HAProxy scrape must preserve previous counter baseline, got %#v", backend.last)
	}
}

func TestHAProxyApplyKeepsRuleNameMapWhenReloadFails(t *testing.T) {
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
	backend.rememberRuleNames(haproxyRuleNames([]agent.RuleConfig{{ID: "old-rule"}}))
	err := backend.Apply(context.Background(), agent.ConfigSnapshot{
		ConfigVersion: 2,
		Rules:         []agent.RuleConfig{anyRule("new-rule", ModeHAProxy, domain.ProtocolTCP, 10000)},
	})
	if err == nil {
		t.Fatalf("expected HAProxy reload to fail")
	}
	if got := backend.ruleIDForBackend("be_old_rule"); got != "old-rule" {
		t.Fatalf("failed reload must preserve old backend rule map, got %q", got)
	}
	if got := backend.ruleIDForBackend("be_new_rule"); got == "new-rule" {
		t.Fatalf("failed reload must not commit new backend rule map")
	}
}

func TestManagedTrafficDeltasTreatsLowerCountersAsReset(t *testing.T) {
	deltas := managedTrafficDeltas(
		map[string]managedTrafficSnapshot{"rule-1": {
			UploadBytes:         10,
			DownloadBytes:       20,
			TCPConnectionEvents: 1,
			UDPPackets:          2,
		}},
		map[string]managedTrafficSnapshot{"rule-1": {
			UploadBytes:         100,
			DownloadBytes:       200,
			TCPConnectionEvents: 9,
			UDPPackets:          8,
		}},
	)
	if len(deltas) != 1 {
		t.Fatalf("expected reset counters to emit one delta, got %#v", deltas)
	}
	if deltas[0].UploadBytes != 10 || deltas[0].DownloadBytes != 20 || deltas[0].TCPConnections != 1 || deltas[0].UDPPackets != 2 {
		t.Fatalf("expected reset counters to be counted as new traffic, got %#v", deltas[0])
	}
}

func TestNFTablesAgentMetricsParsesOwnedCounters(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{
		{output: []byte(`table inet prism_test {
  chain prerouting {
    tcp dport 10000 counter packets 2 bytes 128 dnat ip to 1.1.1.1:443 comment "prism rule=rule-1 version=1 hash=abc"
  }
  chain postrouting {
    ip daddr 1.1.1.1 tcp dport 443 counter packets 2 bytes 128 masquerade comment "prism rule=rule-1 version=1 hash=abc"
  }
}`)},
		{output: []byte(`table inet prism_test {
  chain prerouting {
    tcp dport 10000 counter packets 3 bytes 512 dnat ip to 1.1.1.1:443 comment "prism rule=rule-1 version=1 hash=abc"
  }
  chain postrouting {
    ip daddr 1.1.1.1 tcp dport 443 counter packets 3 bytes 512 masquerade comment "prism rule=rule-1 version=1 hash=abc"
  }
}`)},
	}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	_ = backend.AgentMetrics()
	payload := backend.AgentMetrics()
	if len(payload.TrafficDeltas) != 1 || payload.TrafficDeltas[0].RuleID != "rule-1" || payload.TrafficDeltas[0].UploadBytes != 384 {
		t.Fatalf("expected nftables counter delta, got %#v", payload.TrafficDeltas)
	}
}

func TestNFTablesAgentMetricsParsesUDPPackets(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{
		{output: []byte(`table inet prism_test {
  chain prerouting {
    udp dport 10000 counter packets 2 bytes 128 dnat ip to 1.1.1.1:443 comment "prism rule=rule-udp version=1 hash=abc"
  }
}`)},
		{output: []byte(`table inet prism_test {
  chain prerouting {
    udp dport 10000 counter packets 5 bytes 512 dnat ip to 1.1.1.1:443 comment "prism rule=rule-udp version=1 hash=abc"
  }
}`)},
	}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	_ = backend.AgentMetrics()
	payload := backend.AgentMetrics()
	if len(payload.TrafficDeltas) != 1 || payload.TrafficDeltas[0].RuleID != "rule-udp" || payload.TrafficDeltas[0].UDPPackets != 3 {
		t.Fatalf("expected nftables udp packet delta, got %#v", payload.TrafficDeltas)
	}
}

func TestNFTablesAgentMetricsPreservesLastCountersWhenScrapeFails(t *testing.T) {
	runner := &recordingCommandRunner{outputs: []commandResult{
		{output: []byte(`table inet prism_test {
  chain prerouting {
    tcp dport 10000 counter packets 2 bytes 128 dnat ip to 1.1.1.1:443 comment "prism rule=rule-1 version=1 hash=abc"
  }
}`)},
		{err: errors.New("stats unavailable")},
		{output: []byte(`table inet prism_test {
  chain prerouting {
    tcp dport 10000 counter packets 3 bytes 512 dnat ip to 1.1.1.1:443 comment "prism rule=rule-1 version=1 hash=abc"
  }
}`)},
	}}
	backend := NewNFTablesBackend(Options{InstanceID: "test", CommandRunner: runner})
	_ = backend.AgentMetrics()
	if payload := backend.AgentMetrics(); len(payload.TrafficDeltas) != 0 {
		t.Fatalf("failed scrape must not emit deltas, got %#v", payload.TrafficDeltas)
	}
	payload := backend.AgentMetrics()
	if len(payload.TrafficDeltas) != 1 || payload.TrafficDeltas[0].UploadBytes != 384 {
		t.Fatalf("next successful scrape must use previous successful baseline, got %#v", payload.TrafficDeltas)
	}
}
