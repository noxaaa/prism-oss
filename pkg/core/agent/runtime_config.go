package agent

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
)

type RuntimeConfig struct {
	AppName             string
	ControlPlaneURL     string
	AgentID             string
	RegistrationToken   string
	AgentCredential     string
	AgentCredentialFile string
	ServiceName         string
	InstallDir          string
	LogLevel            string
	preferRegistration  bool
	credentialFinalized bool
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	return LoadRuntimeConfigFromArgs(nil)
}

func LoadRuntimeConfigFromArgs(args []string) (RuntimeConfig, error) {
	cfg := RuntimeConfig{
		AppName:             os.Getenv("APP_NAME"),
		ControlPlaneURL:     os.Getenv("CONTROL_PLANE_URL"),
		AgentID:             os.Getenv("AGENT_ID"),
		RegistrationToken:   os.Getenv("AGENT_REGISTRATION_TOKEN"),
		AgentCredential:     os.Getenv("AGENT_CREDENTIAL"),
		AgentCredentialFile: os.Getenv("AGENT_CREDENTIAL_FILE"),
		ServiceName:         envOrDefault("AGENT_SERVICE_NAME", "prism-node-agent"),
		InstallDir:          envOrDefault("AGENT_INSTALL_DIR", "/opt/prism-node-agent"),
		LogLevel:            envOrDefault("LOG_LEVEL", "info"),
	}
	if cfg.AgentCredential != "" {
		cfg.credentialFinalized = true
	}
	if len(args) > 0 {
		if args[0] != "install" && args[0] != "run" {
			return RuntimeConfig{}, errors.New("unsupported node-agent command")
		}
		if err := applyInstallFlags(&cfg, args[1:]); err != nil {
			return RuntimeConfig{}, err
		}
	}
	if cfg.AppName == "" {
		return RuntimeConfig{}, errors.New("APP_NAME is required")
	}
	if cfg.AgentCredential == "" && cfg.AgentCredentialFile != "" {
		credential, finalized, err := readCredentialFile(cfg.AgentCredentialFile)
		if err != nil {
			return RuntimeConfig{}, err
		}
		cfg.AgentCredential = credential
		cfg.credentialFinalized = finalized
	}
	if cfg.AgentCredential != "" && !cfg.credentialFinalized && cfg.RegistrationToken != "" {
		cfg.preferRegistration = true
	}
	return cfg, nil
}

func applyInstallFlags(cfg *RuntimeConfig, args []string) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	controlURL := flags.String("control-url", "", "control plane URL")
	registrationToken := flags.String("registration-token", "", "agent registration token")
	agentID := flags.String("agent-id", "", "agent ID")
	credential := flags.String("agent-credential", "", "agent credential")
	credentialFile := flags.String("credential-file", "", "agent credential file")
	serviceName := flags.String("service-name", "", "system service name")
	installDir := flags.String("install-dir", "", "agent installation directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *controlURL != "" {
		cfg.ControlPlaneURL = *controlURL
	}
	if *registrationToken != "" {
		cfg.RegistrationToken = *registrationToken
		cfg.preferRegistration = true
	}
	if *agentID != "" {
		cfg.AgentID = *agentID
	}
	if *credential != "" {
		cfg.AgentCredential = *credential
		cfg.credentialFinalized = true
	}
	if *credentialFile != "" {
		cfg.AgentCredentialFile = *credentialFile
	}
	if *serviceName != "" {
		cfg.ServiceName = *serviceName
	}
	if *installDir != "" {
		cfg.InstallDir = *installDir
	}
	return nil
}

func readCredentialFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	var payload struct {
		AgentCredential       string `json:"agent_credential"`
		RegistrationFinalized *bool  `json:"registration_finalized"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false, err
	}
	if payload.RegistrationFinalized == nil {
		return "", false, errors.New("credential file missing registration_finalized")
	}
	return payload.AgentCredential, *payload.RegistrationFinalized, nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
