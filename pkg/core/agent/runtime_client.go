package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"nhooyr.io/websocket"
)

type ConfigApplier interface {
	Apply(ctx context.Context, snapshot ConfigSnapshot) error
}

type MetricsProvider interface {
	AgentMetrics() MetricsPayload
}

type NodeRuntime struct {
	config               RuntimeConfig
	agentType            string
	applier              ConfigApplier
	bootTime             time.Time
	metricsInterval      time.Duration
	heartbeatInterval    time.Duration
	configMu             sync.RWMutex
	versionMu            sync.RWMutex
	appliedConfigVersion int
	writeMu              sync.Mutex
	logMu                sync.Mutex
	lastErrorLogAt       time.Time
	lastErrorSignature   string
	trafficMu            sync.Mutex
	trafficSpoolLoaded   bool
	trafficPending       []RuleTrafficDelta
	trafficInFlightID    string
	trafficInFlight      []RuleTrafficDelta
	monitorMu            sync.Mutex
	monitorSnapshot      MonitorConfigSnapshot
	monitorLastProbe     map[string]time.Time
}

type runtimeEnvelope struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	SentAt    string          `json:"sent_at"`
	Payload   json.RawMessage `json:"payload"`
}

var runCombinedCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func NewNodeRuntime(config RuntimeConfig, applier ConfigApplier) *NodeRuntime {
	return &NodeRuntime{
		config:            config,
		agentType:         "NODE",
		applier:           applier,
		bootTime:          time.Now().UTC(),
		metricsInterval:   5 * time.Second,
		heartbeatInterval: 5 * time.Second,
		monitorLastProbe:  map[string]time.Time{},
	}
}

func NewMonitorRuntime(config RuntimeConfig) *NodeRuntime {
	return &NodeRuntime{
		config:            config,
		agentType:         "MONITOR",
		bootTime:          time.Now().UTC(),
		metricsInterval:   time.Second,
		heartbeatInterval: 5 * time.Second,
		monitorLastProbe:  map[string]time.Time{},
	}
}

func (runtime *NodeRuntime) Run(ctx context.Context) error {
	if err := runtime.validateStaticConfig(); err != nil {
		return err
	}
	for {
		if err := runtime.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			runtime.logRuntimeError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (runtime *NodeRuntime) runOnce(ctx context.Context) error {
	if err := runtime.validateStaticConfig(); err != nil {
		return err
	}
	token, tokenSource := runtime.authTokenWithSource()
	connectURL := agentWebSocketURL(runtime.getControlPlaneURL())
	conn, response, err := runtime.dialControlPlane(ctx, connectURL, token, tokenSource)
	if err != nil && response != nil && response.StatusCode == http.StatusUnauthorized {
		if fallbackToken, fallbackSource, ok := runtime.authFallbackToken(tokenSource); ok {
			tokenSource = fallbackSource
			conn, response, err = runtime.dialControlPlane(ctx, connectURL, fallbackToken, fallbackSource)
		}
	}
	if err != nil {
		return controlPlaneDialError(connectURL, tokenSource, response, err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	if err := runtime.authenticateConnection(ctx, conn); err != nil {
		return err
	}
	if err := runtime.writeHello(ctx, conn); err != nil {
		return err
	}
	reportDone := make(chan struct{})
	go runtime.reportRuntime(ctx, conn, reportDone)
	defer close(reportDone)
	for {
		envelope, err := runtime.read(ctx, conn)
		if err != nil {
			return err
		}
		switch envelope.Type {
		case "auth_success":
			_ = runtime.handleAuthEnvelope(envelope)
		case "registration_success":
			if err := runtime.handleAuthEnvelope(envelope); err != nil {
				return err
			}
			if err := runtime.writeRegistrationAck(ctx, conn); err != nil {
				return err
			}
		case "registration_finalized":
			if err := runtime.finalizeCredential(); err != nil {
				return err
			}
		case "config_snapshot":
			var snapshot ConfigSnapshot
			if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
				_ = runtime.write(ctx, conn, "config_ack", map[string]any{
					"config_version": snapshot.ConfigVersion,
					"status":         "FAILED",
					"error_message":  "invalid config snapshot",
				})
				continue
			}
			status := "APPLIED"
			errorMessage := ""
			applyErrors := []ConfigApplyErrorDetail{}
			if runtime.applier != nil {
				if err := runtime.applier.Apply(ctx, snapshot); err != nil {
					status = "FAILED"
					errorMessage = err.Error()
					applyErrors = StructuredApplyErrors(err)
				}
			}
			if status == "APPLIED" {
				runtime.setAppliedConfigVersion(snapshot.ConfigVersion)
			}
			if err := runtime.write(ctx, conn, "config_ack", map[string]any{
				"config_version": snapshot.ConfigVersion,
				"status":         status,
				"error_message":  nullableString(errorMessage),
				"errors":         applyErrors,
			}); err != nil {
				return err
			}
		case "monitor_config_snapshot":
			var snapshot MonitorConfigSnapshot
			if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
				_ = runtime.write(ctx, conn, "monitor_config_ack", map[string]any{
					"config_version": snapshot.ConfigVersion,
					"status":         "FAILED",
					"error_message":  "invalid monitor config snapshot",
				})
				continue
			}
			runtime.setMonitorSnapshot(snapshot)
			runtime.setAppliedConfigVersion(snapshot.ConfigVersion)
			if err := runtime.write(ctx, conn, "monitor_config_ack", map[string]any{
				"config_version": snapshot.ConfigVersion,
				"status":         "APPLIED",
				"error_message":  nil,
			}); err != nil {
				return err
			}
		case "agent_update_request":
			var request struct {
				TargetVersion  string `json:"target_version"`
				ReleaseBaseURL string `json:"release_base_url"`
				SHA256SumsURL  string `json:"sha256sums_url"`
			}
			if err := json.Unmarshal(envelope.Payload, &request); err != nil || strings.TrimSpace(request.TargetVersion) == "" {
				_ = runtime.write(ctx, conn, "agent_update_result", map[string]any{"status": "FAILED", "error_message": "invalid agent update request"})
				continue
			}
			if err := runtime.write(ctx, conn, "agent_update_result", map[string]any{"status": "RUNNING", "error_message": nil}); err != nil {
				return err
			}
			if err := runtime.applyAgentUpdate(request.TargetVersion, request.ReleaseBaseURL, request.SHA256SumsURL); err != nil {
				_ = runtime.write(ctx, conn, "agent_update_result", map[string]any{"status": "FAILED", "error_message": err.Error()})
				continue
			}
			if err := runtime.restartAgentService(); err != nil {
				_ = runtime.write(ctx, conn, "agent_update_result", map[string]any{"status": "FAILED", "error_message": err.Error()})
				continue
			}
			// The control plane marks success when the restarted agent reconnects
			// and reports the desired version in hello.
		case "metrics_ack":
			var payload struct {
				TrafficReportID string `json:"traffic_report_id"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
				runtime.acknowledgeTrafficReport(payload.TrafficReportID)
			}
		}
	}
}

func (runtime *NodeRuntime) dialControlPlane(ctx context.Context, connectURL string, token string, tokenSource string) (*websocket.Conn, *http.Response, error) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("X-Agent-Type", runtime.getAgentType())
	headers.Set("X-Agent-Version", buildinfo.Version)
	return websocket.Dial(ctx, connectURL, &websocket.DialOptions{HTTPHeader: headers})
}

func (runtime *NodeRuntime) authFallbackToken(tokenSource string) (string, string, bool) {
	switch tokenSource {
	case "registration":
		if credential := runtime.getAgentCredential(); credential != "" {
			return credential, "credential", true
		}
	case "credential":
		if registrationToken := runtime.getRegistrationToken(); registrationToken != "" {
			return registrationToken, "registration", true
		}
	}
	return "", "", false
}

func controlPlaneDialError(connectURL string, tokenSource string, response *http.Response, err error) error {
	if response != nil {
		return fmt.Errorf("connect %s with %s token failed: http %d: %w", connectURL, tokenSource, response.StatusCode, err)
	}
	return fmt.Errorf("connect %s with %s token failed: %w", connectURL, tokenSource, err)
}

func (runtime *NodeRuntime) logRuntimeError(err error) {
	if err == nil {
		return
	}
	signature := err.Error()
	now := time.Now()
	runtime.logMu.Lock()
	defer runtime.logMu.Unlock()
	if signature == runtime.lastErrorSignature && now.Sub(runtime.lastErrorLogAt) < 30*time.Second {
		return
	}
	runtime.lastErrorSignature = signature
	runtime.lastErrorLogAt = now
	log.Printf("%s node-agent connection failed: %v", runtime.getAppName(), err)
}

func (runtime *NodeRuntime) authenticateConnection(ctx context.Context, conn *websocket.Conn) error {
	authEnvelope, err := runtime.read(ctx, conn)
	if err != nil {
		return err
	}
	if err := runtime.handleAuthEnvelope(authEnvelope); err != nil {
		return err
	}
	if authEnvelope.Type != "registration_success" {
		return nil
	}
	if err := runtime.writeRegistrationAck(ctx, conn); err != nil {
		return err
	}
	finalizedEnvelope, err := runtime.read(ctx, conn)
	if err != nil {
		return err
	}
	if finalizedEnvelope.Type != "registration_finalized" {
		return errors.New("expected registration_finalized before hello")
	}
	return runtime.finalizeCredential()
}

func (runtime *NodeRuntime) writeHello(ctx context.Context, conn *websocket.Conn) error {
	return runtime.write(ctx, conn, "hello", map[string]any{
		"agent_id":               runtime.getAgentID(),
		"agent_type":             runtime.getAgentType(),
		"hostname":               runtime.getAgentID(),
		"boot_time":              runtime.bootTime.Format(time.RFC3339Nano),
		"applied_config_version": runtime.getAppliedConfigVersion(),
		"agent_version":          buildinfo.Version,
		"agent_commit":           buildinfo.Commit,
		"agent_build_time":       buildinfo.BuildTime,
	})
}

func (runtime *NodeRuntime) applyAgentUpdate(targetVersion string, releaseBaseURL string, sha256SumsURL string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{
		"upgrade",
		"--version", targetVersion,
		"--service-name", runtime.config.ServiceName,
		"--install-dir", runtime.config.InstallDir,
		"--no-restart",
	}
	if strings.TrimSpace(releaseBaseURL) != "" {
		args = append(args, "--release-base-url", strings.TrimSpace(releaseBaseURL))
	}
	if strings.TrimSpace(sha256SumsURL) != "" {
		args = append(args, "--sha256sums-url", strings.TrimSpace(sha256SumsURL))
	}
	cmd := exec.Command(executable, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func (runtime *NodeRuntime) restartAgentService() error {
	serviceName := strings.TrimSpace(runtime.config.ServiceName)
	if serviceName == "" {
		return errors.New("node-agent service name is required for self-restart")
	}
	output, err := runCombinedCommand("systemctl", "--no-block", "restart", serviceName+".service")
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return errors.New("restart node-agent service: " + message)
	}
	return nil
}

func (runtime *NodeRuntime) writeRegistrationAck(ctx context.Context, conn *websocket.Conn) error {
	return runtime.write(ctx, conn, "registration_ack", map[string]any{
		"agent_id":   runtime.getAgentID(),
		"agent_type": runtime.getAgentType(),
		"status":     "PERSISTED",
	})
}

func (runtime *NodeRuntime) read(ctx context.Context, conn *websocket.Conn) (runtimeEnvelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return runtimeEnvelope{}, err
	}
	var envelope runtimeEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return runtimeEnvelope{}, err
	}
	return envelope, nil
}

func (runtime *NodeRuntime) handleAuthEnvelope(envelope runtimeEnvelope) error {
	switch envelope.Type {
	case "auth_success":
		var payload struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return err
		}
		if payload.AgentID != "" {
			runtime.setAgentID(payload.AgentID)
		}
		if runtime.needsCredentialFinalization() {
			return runtime.finalizeCredential()
		}
		return nil
	case "registration_success":
		var payload struct {
			AgentCredential         string `json:"agent_credential"`
			AgentID                 string `json:"agent_id"`
			AgentCredentialFileHint string `json:"agent_credential_file_hint"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return err
		}
		if payload.AgentCredential == "" {
			return errors.New("registration response missing agent credential")
		}
		runtime.ensureCredentialFile(payload.AgentCredentialFileHint)
		if payload.AgentID != "" {
			runtime.setAgentID(payload.AgentID)
		}
		if err := runtime.persistCredential(payload.AgentCredential, false); err != nil {
			return err
		}
		runtime.setAgentCredential(payload.AgentCredential, false)
		return nil
	default:
		return errors.New("expected auth_success or registration_success before runtime reporting")
	}
}

func (runtime *NodeRuntime) validateStaticConfig() error {
	if runtime.getControlPlaneURL() == "" || runtime.authToken() == "" {
		return errors.New("CONTROL_PLANE_URL and agent credential or registration token are required")
	}
	return nil
}

func (runtime *NodeRuntime) persistCredential(credential string, registrationFinalized bool) error {
	credentialFile := runtime.getAgentCredentialFile()
	if credentialFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(credentialFile), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{
		"agent_credential":       credential,
		"registration_finalized": registrationFinalized,
	})
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(credentialFile), ".agent-credential-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, credentialFile); err != nil {
		return err
	}
	return os.Chmod(credentialFile, 0o600)
}

func (runtime *NodeRuntime) reportRuntime(ctx context.Context, conn *websocket.Conn, done <-chan struct{}) {
	metricsTicker := time.NewTicker(runtime.metricsInterval)
	heartbeatTicker := time.NewTicker(runtime.heartbeatInterval)
	defer metricsTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-heartbeatTicker.C:
			_ = runtime.write(ctx, conn, "heartbeat", map[string]any{
				"agent_id":               runtime.getAgentID(),
				"agent_type":             runtime.getAgentType(),
				"applied_config_version": runtime.getAppliedConfigVersion(),
			})
		case <-metricsTicker.C:
			_ = runtime.write(ctx, conn, "metrics", runtime.collectMetrics())
			if runtime.getAgentType() == "MONITOR" {
				if results := runtime.collectDueHealthResults(ctx); len(results) > 0 {
					_ = runtime.write(ctx, conn, "health_results", map[string]any{"results": results})
				}
			}
		}
	}
}

func (runtime *NodeRuntime) collectMetrics() MetricsPayload {
	var metrics MetricsPayload
	if provider, ok := runtime.applier.(MetricsProvider); ok {
		metrics = provider.AgentMetrics()
	}
	if samples, err := cpu.Percent(0, false); err == nil && len(samples) > 0 {
		metrics.CPUPercent = samples[0]
	}
	if memory, err := mem.VirtualMemory(); err == nil && memory != nil {
		metrics.RAMUsedBytes = memory.Used
		metrics.RAMTotalBytes = memory.Total
	}
	metrics.UptimeSeconds = int64(time.Since(runtime.bootTime).Seconds())
	metrics.BootTime = runtime.bootTime.Format(time.RFC3339Nano)
	metrics.AppliedConfigVersion = runtime.getAppliedConfigVersion()
	runtime.queueTrafficDeltas(metrics.TrafficDeltas)
	metrics.TrafficDeltas = nil
	runtime.attachTrafficReport(&metrics)
	return metrics
}

func (runtime *NodeRuntime) getControlPlaneURL() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.ControlPlaneURL
}

func (runtime *NodeRuntime) getAppName() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.AppName
}

func (runtime *NodeRuntime) getAgentType() string {
	if runtime.agentType == "" {
		return "NODE"
	}
	return runtime.agentType
}

func (runtime *NodeRuntime) getAgentID() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.AgentID
}

func (runtime *NodeRuntime) setAgentID(agentID string) {
	runtime.configMu.Lock()
	defer runtime.configMu.Unlock()
	runtime.config.AgentID = agentID
}

func (runtime *NodeRuntime) getRegistrationToken() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.RegistrationToken
}

func (runtime *NodeRuntime) getAgentCredential() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.AgentCredential
}

func (runtime *NodeRuntime) setAgentCredential(credential string, finalized bool) {
	runtime.configMu.Lock()
	defer runtime.configMu.Unlock()
	runtime.config.AgentCredential = credential
	runtime.config.credentialFinalized = finalized
}

func (runtime *NodeRuntime) getAgentCredentialFile() string {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.AgentCredentialFile
}

func (runtime *NodeRuntime) ensureCredentialFile(hint string) {
	runtime.configMu.Lock()
	defer runtime.configMu.Unlock()
	if runtime.config.AgentCredentialFile != "" {
		return
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "agent-credential.json"
	}
	runtime.config.AgentCredentialFile = hint
}

func (runtime *NodeRuntime) authToken() string {
	token, _ := runtime.authTokenWithSource()
	return token
}

func (runtime *NodeRuntime) authTokenWithSource() (string, string) {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	if runtime.config.AgentCredential != "" && (runtime.config.credentialFinalized || !runtime.config.preferRegistration || runtime.config.RegistrationToken == "") {
		return runtime.config.AgentCredential, "credential"
	}
	if runtime.config.RegistrationToken != "" {
		return runtime.config.RegistrationToken, "registration"
	}
	return runtime.config.AgentCredential, "credential"
}

func (runtime *NodeRuntime) finalizeCredential() error {
	credential := runtime.getAgentCredential()
	if credential == "" {
		return nil
	}
	if err := runtime.persistCredential(credential, true); err != nil {
		return err
	}
	runtime.configMu.Lock()
	defer runtime.configMu.Unlock()
	runtime.config.credentialFinalized = true
	runtime.config.RegistrationToken = ""
	runtime.config.preferRegistration = false
	return nil
}

func (runtime *NodeRuntime) needsCredentialFinalization() bool {
	runtime.configMu.RLock()
	defer runtime.configMu.RUnlock()
	return runtime.config.AgentCredential != "" && !runtime.config.credentialFinalized
}

func (runtime *NodeRuntime) getAppliedConfigVersion() int {
	runtime.versionMu.RLock()
	defer runtime.versionMu.RUnlock()
	return runtime.appliedConfigVersion
}

func (runtime *NodeRuntime) setAppliedConfigVersion(version int) {
	runtime.versionMu.Lock()
	defer runtime.versionMu.Unlock()
	runtime.appliedConfigVersion = version
}

func (runtime *NodeRuntime) write(ctx context.Context, conn *websocket.Conn, messageType string, payload any) error {
	data, err := json.Marshal(map[string]any{
		"type":       messageType,
		"message_id": messageType + "_" + time.Now().UTC().Format("20060102150405.000000000"),
		"sent_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"payload":    payload,
	})
	if err != nil {
		return err
	}
	runtime.writeMu.Lock()
	defer runtime.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageText, data)
}

func agentWebSocketURL(controlPlaneURL string) string {
	base := strings.TrimRight(controlPlaneURL, "/")
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://") + "/agent/v1/connect"
	}
	if strings.HasPrefix(base, "http://") {
		return "ws://" + strings.TrimPrefix(base, "http://") + "/agent/v1/connect"
	}
	return base + "/agent/v1/connect"
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
