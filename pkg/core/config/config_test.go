package config

import "testing"

func TestLoadControlPlaneRequiresAppName(t *testing.T) {
	t.Setenv("APP_NAME", "")

	_, err := LoadControlPlane()
	if err == nil {
		t.Fatalf("expected missing APP_NAME error")
	}
}

func TestLoadControlPlaneReadsRuntimeDisplayNameAndURLs(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_WEB_URL", "http://localhost:3000")
	t.Setenv("CONTROL_PLANE_URL", "http://localhost:8080")
	t.Setenv("AGENT_TOKEN_SIGNING_SECRET", "agent-token-secret-32-bytes")
	t.Setenv("AGENT_RELEASE_VERSION", "v1.2.3")

	cfg, err := LoadControlPlane()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AppName != "Runtime Name" {
		t.Fatalf("expected runtime app name, got %q", cfg.AppName)
	}
	if cfg.ControlPlaneURL != "http://localhost:8080" {
		t.Fatalf("expected control URL from env, got %q", cfg.ControlPlaneURL)
	}
	if cfg.AgentReleaseVersion != "v1.2.3" {
		t.Fatalf("expected agent release version from env, got %q", cfg.AgentReleaseVersion)
	}
}

func TestLoadControlPlaneDefaultsAgentReleaseVersionToLatest(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "http://localhost:8080")
	t.Setenv("AGENT_TOKEN_SIGNING_SECRET", "agent-token-secret-32-bytes")
	t.Setenv("AGENT_RELEASE_VERSION", "")

	cfg, err := LoadControlPlane()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentReleaseVersion != "latest" {
		t.Fatalf("expected default agent release version latest, got %q", cfg.AgentReleaseVersion)
	}
}

func TestLoadControlPlaneRequiresAgentRegistrationConfig(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "")
	t.Setenv("AGENT_TOKEN_SIGNING_SECRET", "agent-token-secret-32-bytes")
	if _, err := LoadControlPlane(); err == nil {
		t.Fatalf("expected missing CONTROL_PLANE_URL error")
	}

	t.Setenv("CONTROL_PLANE_URL", "http://localhost:8080")
	t.Setenv("AGENT_TOKEN_SIGNING_SECRET", "")
	if _, err := LoadControlPlane(); err == nil {
		t.Fatalf("expected missing AGENT_TOKEN_SIGNING_SECRET error")
	}
}
