package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeRuntimeFinalizeCredentialScrubsBootstrapTokens(t *testing.T) {
	tempDir := t.TempDir()
	credentialPath := filepath.Join(tempDir, "credential.json")
	configPath := filepath.Join(tempDir, "agent.env")
	if err := os.WriteFile(configPath, []byte("APP_NAME='Runtime App'\nAGENT_REGISTRATION_TOKEN='manual-token'\nAGENT_ENROLLMENT_TOKEN='enroll-token'\nCONTROL_PLANE_URL='http://127.0.0.1'\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ConfigFile:          configPath,
		AgentCredential:     "new-secret",
		AgentCredentialFile: credentialPath,
	}, nil)
	if err := runtime.finalizeCredential(); err != nil {
		t.Fatalf("finalize credential: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "manual-token") || strings.Contains(text, "enroll-token") {
		t.Fatalf("expected bootstrap tokens to be scrubbed, got %s", text)
	}
	if !strings.Contains(text, "AGENT_REGISTRATION_TOKEN=''") || !strings.Contains(text, "AGENT_ENROLLMENT_TOKEN=''") {
		t.Fatalf("expected empty bootstrap token lines, got %s", text)
	}
}

func TestNodeRuntimePrefersPendingCredentialBeforeEnrollmentToken(t *testing.T) {
	tempDir := t.TempDir()
	credentialPath := filepath.Join(tempDir, "agent-credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"pending-credential","registration_finalized":false}`), 0o600); err != nil {
		t.Fatalf("write pending credential: %v", err)
	}
	credential, finalized, err := readCredentialFile(credentialPath)
	if err != nil {
		t.Fatalf("read pending credential: %v", err)
	}
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ControlPlaneURL:     "http://127.0.0.1:8080",
		EnrollmentToken:     "fresh-enrollment-token",
		AgentCredential:     credential,
		AgentCredentialFile: credentialPath,
		preferRegistration:  true,
		credentialFinalized: finalized,
	}, nil)
	token, source := runtime.authTokenWithSource()
	if source != "credential" || token != "pending-credential" {
		t.Fatalf("expected pending credential to be tried first, got source=%q token=%q", source, token)
	}
}
