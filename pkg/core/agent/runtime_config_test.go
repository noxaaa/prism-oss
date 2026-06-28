package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRuntimeConfigRequiresAppName(t *testing.T) {
	t.Setenv("APP_NAME", "")

	_, err := LoadRuntimeConfig()
	if err == nil {
		t.Fatalf("expected missing APP_NAME error")
	}
}

func TestLoadRuntimeConfigReadsOnlyAgentRuntimeEnv(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "http://localhost:8080")
	t.Setenv("AGENT_ID", "agent-01")
	t.Setenv("AGENT_REGISTRATION_TOKEN", "registration-token")
	t.Setenv("AGENT_CREDENTIAL", "credential-token")
	t.Setenv("AGENT_CREDENTIAL_FILE", "/var/lib/agent/credential.json")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DATABASE_URL", "postgres://ignored")
	t.Setenv("QUEUE_REDIS_URL", "redis://queue")
	t.Setenv("CACHE_REDIS_URL", "redis://cache")
	t.Setenv("CONTROL_PLANE_INTERNAL_JWT_SECRET", "control-secret")
	t.Setenv("DNS_SECRET_ENCRYPTION_KEY", "dns-secret")

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.AppName != "Runtime Name" {
		t.Fatalf("expected runtime app name, got %q", cfg.AppName)
	}
	if cfg.ControlPlaneURL != "http://localhost:8080" {
		t.Fatalf("expected control URL from env, got %q", cfg.ControlPlaneURL)
	}
	if cfg.AgentID != "agent-01" {
		t.Fatalf("expected agent id from env, got %q", cfg.AgentID)
	}
	if cfg.RegistrationToken != "registration-token" {
		t.Fatalf("expected registration token from env, got %q", cfg.RegistrationToken)
	}
	if cfg.AgentCredential != "credential-token" {
		t.Fatalf("expected agent credential from env, got %q", cfg.AgentCredential)
	}
	if !cfg.credentialFinalized {
		t.Fatalf("expected direct env credential to be treated as finalized")
	}
	if cfg.AgentCredentialFile != "/var/lib/agent/credential.json" {
		t.Fatalf("expected credential file from env, got %q", cfg.AgentCredentialFile)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected log level from env, got %q", cfg.LogLevel)
	}

	forbiddenFields := []string{
		"DatabaseURL",
		"QueueRedisURL",
		"CacheRedisURL",
		"ControlPlaneInternalKey",
		"AgentTokenSigningSecret",
		"DNSSecretEncryptionKey",
	}
	cfgType := reflect.TypeOf(cfg)
	for _, field := range forbiddenFields {
		if _, ok := cfgType.FieldByName(field); ok {
			t.Fatalf("agent runtime config must not expose control-plane field %s", field)
		}
	}
}

func TestLoadRuntimeConfigReadsInstallFlags(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "")
	t.Setenv("AGENT_REGISTRATION_TOKEN", "")

	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "https://control.example.com",
		"--registration-token", "registration-token",
		"--agent-id", "node_1",
		"--credential-file", "/var/lib/agent/credential.json",
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}
	if cfg.ControlPlaneURL != "https://control.example.com" {
		t.Fatalf("expected control URL from install flag, got %q", cfg.ControlPlaneURL)
	}
	if cfg.RegistrationToken != "registration-token" {
		t.Fatalf("expected registration token from install flag, got %q", cfg.RegistrationToken)
	}
	if !cfg.preferRegistration {
		t.Fatalf("expected install registration token to be preferred until a credential is finalized")
	}
	if cfg.AgentID != "node_1" {
		t.Fatalf("expected agent id from install flag, got %q", cfg.AgentID)
	}
	if cfg.AgentCredentialFile != "/var/lib/agent/credential.json" {
		t.Fatalf("expected credential file from install flag, got %q", cfg.AgentCredentialFile)
	}
}

func TestLoadRuntimeConfigReadsEnrollmentInstallFlag(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "")
	t.Setenv("AGENT_REGISTRATION_TOKEN", "")
	t.Setenv("AGENT_ENROLLMENT_TOKEN", "")

	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "https://control.example.com",
		"--enrollment-token", "enrollment-token",
		"--credential-file", "/var/lib/agent/credential.json",
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}
	if cfg.EnrollmentToken != "enrollment-token" {
		t.Fatalf("expected enrollment token from install flag, got %q", cfg.EnrollmentToken)
	}
	if !cfg.preferRegistration {
		t.Fatalf("expected install enrollment token to be preferred until a credential is finalized")
	}
}

func TestLoadRuntimeConfigRejectsRegistrationAndEnrollmentTokensTogether(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("CONTROL_PLANE_URL", "https://control.example.com")
	t.Setenv("AGENT_REGISTRATION_TOKEN", "registration-token")
	t.Setenv("AGENT_ENROLLMENT_TOKEN", "enrollment-token")

	_, err := LoadRuntimeConfig()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive token error, got %v", err)
	}
}

func TestLoadRuntimeConfigDefaultsLogLevel(t *testing.T) {
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level, got %q", cfg.LogLevel)
	}
}

func TestLoadRuntimeConfigReadsCredentialFileWhenEnvCredentialIsEmpty(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"file-credential","registration_finalized":true}`), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("AGENT_CREDENTIAL", "")
	t.Setenv("AGENT_CREDENTIAL_FILE", credentialPath)

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.AgentCredential != "file-credential" {
		t.Fatalf("expected credential from file, got %q", cfg.AgentCredential)
	}
	if !cfg.credentialFinalized {
		t.Fatalf("expected finalized credential file state")
	}
}

func TestLoadRuntimeConfigRejectsCredentialFileWithoutFinalizedState(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"file-credential"}`), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("AGENT_CREDENTIAL", "")
	t.Setenv("AGENT_CREDENTIAL_FILE", credentialPath)

	_, err := LoadRuntimeConfig()
	if err == nil {
		t.Fatalf("expected missing finalized state to be rejected")
	}
	if !strings.Contains(err.Error(), "registration_finalized") {
		t.Fatalf("expected registration_finalized error, got %v", err)
	}
}

func TestLoadRuntimeConfigReadsPendingCredentialFileState(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"pending-file-credential","registration_finalized":false}`), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("AGENT_CREDENTIAL", "")
	t.Setenv("AGENT_CREDENTIAL_FILE", credentialPath)

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.AgentCredential != "pending-file-credential" {
		t.Fatalf("expected pending credential from file, got %q", cfg.AgentCredential)
	}
	if cfg.credentialFinalized {
		t.Fatalf("expected pending credential file state")
	}
}

func TestLoadRuntimeConfigPrefersEnvRegistrationTokenWhenCredentialFileIsPending(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"pending-file-credential","registration_finalized":false}`), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime Name")
	t.Setenv("AGENT_REGISTRATION_TOKEN", "registration-token")
	t.Setenv("AGENT_CREDENTIAL", "")
	t.Setenv("AGENT_CREDENTIAL_FILE", credentialPath)

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.AgentCredential != "pending-file-credential" {
		t.Fatalf("expected pending credential from file, got %q", cfg.AgentCredential)
	}
	if cfg.credentialFinalized {
		t.Fatalf("expected pending credential file state")
	}
	if !cfg.preferRegistration {
		t.Fatalf("expected env registration token to be preferred while credential file is pending")
	}
}
