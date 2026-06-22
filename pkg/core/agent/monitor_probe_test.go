package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestICMPHealthProbeHonorsTimeout(t *testing.T) {
	previousRunner := runMonitorProbeCommand
	defer func() {
		runMonitorProbeCommand = previousRunner
	}()
	var commandName string
	var commandArgs []string
	runMonitorProbeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commandName = name
		commandArgs = append([]string(nil), args...)
		return []byte("ok"), nil
	}

	status, err := runHealthProbe(context.Background(), MonitorHealthCheck{
		ProbeType:      "ICMP",
		TimeoutSeconds: 7,
	}, MonitorHealthTarget{Host: "192.0.2.10"}, 7*time.Second)
	if err != nil {
		t.Fatalf("expected successful probe, got %v", err)
	}
	if status != "ONLINE" {
		t.Fatalf("expected ONLINE, got %s", status)
	}
	if commandName != "ping" {
		t.Fatalf("expected ping command, got %q", commandName)
	}
	wantArgs := []string{"-c", "1", "-W", "7", "192.0.2.10"}
	if !reflect.DeepEqual(commandArgs, wantArgs) {
		t.Fatalf("unexpected ping args %#v, want %#v", commandArgs, wantArgs)
	}
}

func TestICMPHealthProbeRoundsSubsecondTimeoutUp(t *testing.T) {
	if got := pingTimeoutSeconds(1500 * time.Millisecond); got != 2 {
		t.Fatalf("expected subsecond remainder to round up to 2, got %d", got)
	}
	if got := pingTimeoutSeconds(0); got != 1 {
		t.Fatalf("expected minimum timeout 1, got %d", got)
	}
}

func TestICMPHealthProbeReportsCommandOutput(t *testing.T) {
	previousRunner := runMonitorProbeCommand
	defer func() {
		runMonitorProbeCommand = previousRunner
	}()
	runMonitorProbeCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("timeout"), errors.New("exit status 1")
	}

	status, err := runICMPHealthProbe(context.Background(), "192.0.2.10", time.Second)
	if err == nil {
		t.Fatalf("expected probe error")
	}
	if status != "OFFLINE" {
		t.Fatalf("expected OFFLINE, got %s", status)
	}
}
