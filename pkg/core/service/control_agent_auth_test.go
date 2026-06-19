package service

import (
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
)

func TestNodeInstallCommandUsesSudoForServiceInstall(t *testing.T) {
	service := NewControlServiceWithOptions(nil, ControlServiceOptions{
		AppName:             "OSS Control Console",
		ControlPlaneURL:     "http://control.example:8080",
		AgentReleaseVersion: "v1.2.3",
	})
	command := service.installCommand("NODE", "registration-token")
	for _, required := range []string{
		"sudo env APP_NAME='OSS Control Console' sh \"$tmp\"",
		"--version 'v1.2.3'",
		"--control-url 'http://control.example:8080'",
		"--registration-token 'registration-token'",
		"--credential-file agent-credential.json",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("node install command missing %q: %s", required, command)
		}
	}
}

func TestNodeInstallCommandPinsLatestToControlPlaneBuildVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "v9.9.9"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	service := NewControlServiceWithOptions(nil, ControlServiceOptions{
		AppName:             "OSS Control Console",
		ControlPlaneURL:     "http://control.example:8080",
		AgentReleaseVersion: "latest",
	})
	command := service.installCommand("NODE", "registration-token")
	for _, required := range []string{
		"https://github.com/noxaaa/prism-oss/releases/download/v9.9.9/install-node-agent.sh",
		"--version 'v9.9.9'",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("node install command missing %q: %s", required, command)
		}
	}
	for _, forbidden := range []string{
		"https://github.com/noxaaa/prism-oss/releases/latest/download/install-node-agent.sh",
		"--version 'latest'",
	} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("node install command must not contain %q: %s", forbidden, command)
		}
	}
}

func TestNodeInstallCommandKeepsLatestForDevBuilds(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "dev"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	service := NewControlServiceWithOptions(nil, ControlServiceOptions{
		AppName:             "OSS Control Console",
		ControlPlaneURL:     "http://control.example:8080",
		AgentReleaseVersion: "latest",
	})
	command := service.installCommand("NODE", "registration-token")
	for _, required := range []string{
		"https://github.com/noxaaa/prism-oss/releases/latest/download/install-node-agent.sh",
		"--version 'latest'",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("node install command missing %q: %s", required, command)
		}
	}
}
