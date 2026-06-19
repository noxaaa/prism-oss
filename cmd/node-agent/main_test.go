package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestExtractRunConfigFilePassesRuntimeFlagsThrough(t *testing.T) {
	configFile, runtimeArgs, err := extractRunConfigFile([]string{
		"--config", "/etc/prism-node-agent/agent.env",
		"--control-url", "https://control.example.test",
		"--registration-token", "registration-token",
		"--credential-file", "/var/lib/prism-node-agent/agent-credential.json",
	})
	if err != nil {
		t.Fatalf("extract run config: %v", err)
	}
	if configFile != "/etc/prism-node-agent/agent.env" {
		t.Fatalf("config file = %q", configFile)
	}
	want := []string{
		"--control-url", "https://control.example.test",
		"--registration-token", "registration-token",
		"--credential-file", "/var/lib/prism-node-agent/agent-credential.json",
	}
	if !reflect.DeepEqual(runtimeArgs, want) {
		t.Fatalf("runtime args = %#v, want %#v", runtimeArgs, want)
	}
}

func TestExtractRunConfigFileSupportsEqualsSyntax(t *testing.T) {
	configFile, runtimeArgs, err := extractRunConfigFile([]string{
		"--control-url", "https://control.example.test",
		"--config=/etc/prism-node-agent/agent.env",
		"-config=/etc/prism-node-agent/override.env",
	})
	if err != nil {
		t.Fatalf("extract run config: %v", err)
	}
	if configFile != "/etc/prism-node-agent/override.env" {
		t.Fatalf("config file = %q", configFile)
	}
	want := []string{"--control-url", "https://control.example.test"}
	if !reflect.DeepEqual(runtimeArgs, want) {
		t.Fatalf("runtime args = %#v, want %#v", runtimeArgs, want)
	}
}

func TestExtractRunConfigFileRejectsMissingValue(t *testing.T) {
	_, _, err := extractRunConfigFile([]string{"--config"})
	if err == nil {
		t.Fatalf("expected missing config value error")
	}
	if !strings.Contains(err.Error(), "--config") {
		t.Fatalf("expected --config error, got %v", err)
	}
}

func TestCopyFileReplacesTargetWithoutTruncatingOpenFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacing an open file is platform-specific")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("new binary"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	openTarget, err := os.Open(target)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer func() { _ = openTarget.Close() }()

	if err := copyFile(source, target, 0o755); err != nil {
		t.Fatalf("copy file: %v", err)
	}

	oldContent, err := io.ReadAll(openTarget)
	if err != nil {
		t.Fatalf("read open target handle: %v", err)
	}
	if string(oldContent) != "old binary" {
		t.Fatalf("open target handle content = %q, want old binary", oldContent)
	}
	newContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(newContent) != "new binary" {
		t.Fatalf("replaced target content = %q, want new binary", newContent)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat replaced target: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("target mode = %v, want 0755", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".target.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}
