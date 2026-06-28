package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"

	"nhooyr.io/websocket"
)

func TestNodeRuntimeAppliesConfigSnapshotAndAcknowledges(t *testing.T) {
	applier := &recordingApplier{}
	ackReceived := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agent/v1/connect" {
			http.NotFound(response, request)
			return
		}
		if got := request.Header.Get("Authorization"); got != "Bearer registration-token" {
			t.Errorf("unexpected authorization header %q", got)
		}
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_credential": "long-lived-credential",
		})
		ack := readRuntimeTestEnvelope(t, request.Context(), conn)
		if ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_finalized", map[string]any{
			"agent_id": "node_1",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello after registration finalization, got %#v", hello)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "config_snapshot", ConfigSnapshot{
			NodeID:        "node_1",
			ConfigVersion: 3,
			Rules: []RuleConfig{
				{
					ID:        "rule_tcp",
					Enabled:   true,
					Protocol:  domain.ProtocolTCP,
					ListenIP:  "127.0.0.1",
					Port:      10000,
					MatchType: "ANY_INBOUND",
				},
			},
		})
		configAck := readRuntimeTestEnvelope(t, request.Context(), conn)
		if configAck.Type != "config_ack" {
			t.Errorf("expected config_ack, got %#v", configAck)
			return
		}
		var payload struct {
			ConfigVersion int    `json:"config_version"`
			Status        string `json:"status"`
		}
		if err := json.Unmarshal(configAck.Payload, &payload); err != nil {
			t.Errorf("decode ack payload: %v", err)
			return
		}
		if payload.Status != "APPLIED" {
			t.Errorf("expected APPLIED ack, got %#v", payload)
			return
		}
		ackReceived <- payload.ConfigVersion
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ControlPlaneURL:     server.URL,
		AgentID:             "node_1",
		RegistrationToken:   "registration-token",
		AgentCredentialFile: credentialPath,
	}, applier)
	go func() {
		_ = runtime.Run(ctx)
	}()

	select {
	case version := <-ackReceived:
		if version != 3 {
			t.Fatalf("expected ack version 3, got %d", version)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for config ack")
	}
	cancel()
	applied := applier.snapshot()
	if applied.ConfigVersion != 3 || len(applied.Rules) != 1 || applied.Rules[0].ID != "rule_tcp" {
		t.Fatalf("unexpected applied config: %#v", applied)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read persisted credential: %v", err)
	}
	if !strings.Contains(string(data), "long-lived-credential") {
		t.Fatalf("expected persisted credential file, got %s", string(data))
	}
	if !strings.Contains(string(data), `"registration_finalized":true`) {
		t.Fatalf("expected finalized credential file, got %s", string(data))
	}
	if registrationToken := runtime.getRegistrationToken(); registrationToken != "" {
		t.Fatalf("expected finalized registration to clear token, got %q", registrationToken)
	}
}

func TestNodeRuntimeReportsRestartFailure(t *testing.T) {
	previousRunner := runCombinedCommand
	defer func() {
		runCombinedCommand = previousRunner
	}()
	var commandName string
	var commandArgs []string
	runCombinedCommand = func(name string, args ...string) ([]byte, error) {
		commandName = name
		commandArgs = append([]string(nil), args...)
		return []byte("restart rejected"), errors.New("exit status 1")
	}

	runtime := NewNodeRuntime(RuntimeConfig{ServiceName: "prism-node-agent"}, nil)
	err := runtime.restartAgentService()
	if err == nil {
		t.Fatalf("expected restart failure")
	}
	if !strings.Contains(err.Error(), "restart rejected") {
		t.Fatalf("expected restart output in error, got %v", err)
	}
	if commandName != "systemctl" {
		t.Fatalf("expected systemctl command, got %q", commandName)
	}
	wantArgs := []string{"--no-block", "restart", "prism-node-agent.service"}
	if strings.Join(commandArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("unexpected restart args %#v, want %#v", commandArgs, wantArgs)
	}
}

func TestNodeRuntimeIncludesStructuredApplyErrorsInConfigAck(t *testing.T) {
	applier := &recordingApplier{err: ConfigApplyError{
		Message: "listen tcp 127.0.0.1:443: bind: address already in use",
		Errors: []ConfigApplyErrorDetail{
			{
				Code:     "LISTENER_BIND_FAILED",
				RuleIDs:  []string{"rule_https"},
				Protocol: domain.ProtocolTCP,
				ListenIP: "127.0.0.1",
				Port:     443,
				Message:  "bind: address already in use",
			},
		},
	}}
	ackReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "auth_success", map[string]any{
			"agent_id": "node_1",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "config_snapshot", ConfigSnapshot{
			NodeID:        "node_1",
			ConfigVersion: 7,
			Rules: []RuleConfig{
				{ID: "rule_https", Enabled: true, Protocol: domain.ProtocolTCP, ListenIP: "127.0.0.1", Port: 443, MatchType: "ANY_INBOUND"},
			},
		})
		configAck := readRuntimeTestEnvelope(t, request.Context(), conn)
		if configAck.Type != "config_ack" {
			t.Errorf("expected config_ack, got %#v", configAck)
			return
		}
		var payload struct {
			ConfigVersion int                      `json:"config_version"`
			Status        string                   `json:"status"`
			ErrorMessage  string                   `json:"error_message"`
			Errors        []ConfigApplyErrorDetail `json:"errors"`
		}
		if err := json.Unmarshal(configAck.Payload, &payload); err != nil {
			t.Errorf("decode config ack: %v", err)
			return
		}
		if payload.ConfigVersion != 7 || payload.Status != "FAILED" || payload.ErrorMessage == "" {
			t.Errorf("unexpected failed ack payload: %#v", payload)
			return
		}
		if len(payload.Errors) != 1 || payload.Errors[0].Code != "LISTENER_BIND_FAILED" || payload.Errors[0].RuleIDs[0] != "rule_https" {
			t.Errorf("expected structured listener bind error in ack, got %#v", payload.Errors)
			return
		}
		ackReceived <- struct{}{}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:         "Runtime App",
		ControlPlaneURL: server.URL,
		AgentCredential: "credential-token",
	}, applier)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour
	go func() {
		_ = runtime.Run(ctx)
	}()

	select {
	case <-ackReceived:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for failed config ack")
	}
}

func TestNodeRuntimeSendsHeartbeatWithAppliedConfigVersion(t *testing.T) {
	type heartbeatPayload struct {
		AgentID              string `json:"agent_id"`
		AppliedConfigVersion int    `json:"applied_config_version"`
	}
	heartbeatReceived := make(chan heartbeatPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		time.Sleep(50 * time.Millisecond)
		writeRuntimeTestEnvelope(t, request.Context(), conn, "auth_success", map[string]any{
			"agent_id": "node_from_auth",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
		for {
			message := readRuntimeTestEnvelope(t, request.Context(), conn)
			if message.Type != "heartbeat" {
				continue
			}
			var payload heartbeatPayload
			if err := json.Unmarshal(message.Payload, &payload); err != nil {
				t.Errorf("decode heartbeat payload: %v", err)
				return
			}
			heartbeatReceived <- payload
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:         "Runtime App",
		ControlPlaneURL: server.URL,
		AgentCredential: "credential-token",
	}, nil)
	runtime.heartbeatInterval = 20 * time.Millisecond
	runtime.metricsInterval = time.Hour
	go func() {
		_ = runtime.Run(ctx)
	}()

	select {
	case payload := <-heartbeatReceived:
		if payload.AgentID != "node_from_auth" {
			t.Fatalf("expected heartbeat agent id from auth response, got %#v", payload)
		}
		if payload.AppliedConfigVersion != 0 {
			t.Fatalf("expected initial applied version 0, got %d", payload.AppliedConfigVersion)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for heartbeat")
	}
}

func TestNodeAndMonitorRuntimeUseOneSecondReportingIntervals(t *testing.T) {
	nodeRuntime := NewNodeRuntime(RuntimeConfig{}, nil)
	if nodeRuntime.metricsInterval != time.Second {
		t.Fatalf("node metrics interval = %s, want %s", nodeRuntime.metricsInterval, time.Second)
	}
	if nodeRuntime.heartbeatInterval != time.Second {
		t.Fatalf("node heartbeat interval = %s, want %s", nodeRuntime.heartbeatInterval, time.Second)
	}
	monitorRuntime := NewMonitorRuntime(RuntimeConfig{})
	if monitorRuntime.metricsInterval != time.Second {
		t.Fatalf("monitor metrics interval = %s, want %s", monitorRuntime.metricsInterval, time.Second)
	}
	if monitorRuntime.heartbeatInterval != time.Second {
		t.Fatalf("monitor heartbeat interval = %s, want %s", monitorRuntime.heartbeatInterval, time.Second)
	}
}

func TestNodeRuntimeMetricsIncludesSystemDetails(t *testing.T) {
	runtime := NewNodeRuntime(RuntimeConfig{}, nil)
	metrics := runtime.collectMetrics()
	if metrics.Architecture != goruntime.GOARCH {
		t.Fatalf("metrics architecture = %q, want %q", metrics.Architecture, goruntime.GOARCH)
	}
	if metrics.CPULogicalCores <= 0 {
		t.Fatalf("expected logical CPU core count, got %d", metrics.CPULogicalCores)
	}
}

func TestNodeRuntimeMetricsUsesCachedSystemDetails(t *testing.T) {
	runtime := NewNodeRuntime(RuntimeConfig{}, nil)
	runtime.staticMetrics = MetricsPayload{CPUModel: "Cached CPU", CPULogicalCores: 12, CPUPhysicalCores: 6, OSName: "cached-os", OSVersion: "cached-version", KernelVersion: "cached-kernel", Architecture: "cached-arch", VirtualizationSystem: "cached-kvm", VirtualizationRole: "guest"}
	metrics := runtime.collectMetrics()
	if metrics.CPUModel != "Cached CPU" || metrics.CPULogicalCores != 12 || metrics.CPUPhysicalCores != 6 {
		t.Fatalf("expected cached CPU details, got %#v", metrics)
	}
	if metrics.OSName != "cached-os" || metrics.OSVersion != "cached-version" || metrics.KernelVersion != "cached-kernel" || metrics.Architecture != "cached-arch" {
		t.Fatalf("expected cached OS details, got %#v", metrics)
	}
	if metrics.VirtualizationSystem != "cached-kvm" || metrics.VirtualizationRole != "guest" {
		t.Fatalf("expected cached virtualization details, got %#v", metrics)
	}
}

func TestMonitorRuntimeRegistersAndReportsMetrics(t *testing.T) {
	type helloPayload struct {
		AgentID   string `json:"agent_id"`
		AgentType string `json:"agent_type"`
	}
	metricsReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer registration-token" {
			t.Errorf("unexpected authorization header %q", got)
		}
		if got := request.Header.Get("X-Agent-Type"); got != "MONITOR" {
			t.Errorf("unexpected agent type header %q", got)
		}
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_id":         "monitor_from_registration",
			"agent_credential": "long-lived-monitor-credential",
		})
		ack := readRuntimeTestEnvelope(t, request.Context(), conn)
		if ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_finalized", map[string]any{
			"agent_id": "monitor_from_registration",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello after registration finalization, got %#v", hello)
			return
		}
		var payload helloPayload
		if err := json.Unmarshal(hello.Payload, &payload); err != nil {
			t.Errorf("decode hello payload: %v", err)
			return
		}
		if payload.AgentType != "MONITOR" {
			t.Errorf("unexpected monitor hello payload %#v", payload)
			return
		}
		for {
			message := readRuntimeTestEnvelope(t, request.Context(), conn)
			if message.Type != "metrics" {
				continue
			}
			metricsReceived <- struct{}{}
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := NewMonitorRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ControlPlaneURL:     server.URL,
		RegistrationToken:   "registration-token",
		AgentCredentialFile: filepath.Join(t.TempDir(), "monitor-credential.json"),
	})
	runtime.metricsInterval = 20 * time.Millisecond
	runtime.heartbeatInterval = time.Hour
	go func() {
		_ = runtime.Run(ctx)
	}()

	select {
	case <-metricsReceived:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for monitor metrics")
	}
}

func TestMonitorRuntimeReportsOneSecondHealthChecksWithDefaultTick(t *testing.T) {
	previousRunner := runMonitorProbeCommand
	defer func() {
		runMonitorProbeCommand = previousRunner
	}()
	runMonitorProbeCommand = func(context.Context, string, ...string) ([]byte, error) {
		return nil, nil
	}

	healthResultsReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_id":         "monitor_1",
			"agent_credential": "long-lived-monitor-credential",
		})
		if ack := readRuntimeTestEnvelope(t, request.Context(), conn); ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_finalized", map[string]any{
			"agent_id": "monitor_1",
		})
		if hello := readRuntimeTestEnvelope(t, request.Context(), conn); hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "monitor_config_snapshot", MonitorConfigSnapshot{
			MonitorID:     "monitor_1",
			ConfigVersion: 1,
			HealthChecks: []MonitorHealthCheck{{
				ID:              "health_1",
				ProbeType:       "ICMP",
				IntervalSeconds: 1,
				TimeoutSeconds:  1,
				ConfigJSON:      "{}",
				Targets: []MonitorHealthTarget{{
					HealthCheckTargetID: "health_target_1",
					TargetID:            "target_1",
					Host:                "127.0.0.1",
				}},
			}},
		})
		if ack := readRuntimeTestEnvelope(t, request.Context(), conn); ack.Type != "monitor_config_ack" {
			t.Errorf("expected monitor_config_ack, got %#v", ack)
			return
		}
		for {
			message := readRuntimeTestEnvelope(t, request.Context(), conn)
			if message.Type != "health_results" {
				continue
			}
			healthResultsReceived <- struct{}{}
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := NewMonitorRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ControlPlaneURL:     server.URL,
		RegistrationToken:   "registration-token",
		AgentCredentialFile: filepath.Join(t.TempDir(), "monitor-credential.json"),
	})
	runtime.heartbeatInterval = time.Hour
	go func() {
		_ = runtime.Run(ctx)
	}()

	select {
	case <-healthResultsReceived:
	case <-time.After(2500 * time.Millisecond):
		t.Fatalf("timed out waiting for one-second health check result")
	}
}

func TestNodeRuntimeAllowsRegistrationTokenWithoutAgentID(t *testing.T) {
	helloReceived := make(chan struct{}, 1)
	workDir := t.TempDir()
	t.Chdir(workDir)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_id":                   "node_from_registration",
			"agent_credential":           "long-lived-credential",
			"agent_credential_file_hint": "agent-credential.json",
		})
		ack := readRuntimeTestEnvelope(t, request.Context(), conn)
		if ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_finalized", map[string]any{
			"agent_id": "node_from_registration",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello after registration finalization, got %#v", hello)
			return
		}
		helloReceived <- struct{}{}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:           "Runtime App",
		ControlPlaneURL:   server.URL,
		RegistrationToken: "registration-token",
	}, nil)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour
	err := runtime.runOnce(ctx)
	if err == nil {
		t.Fatalf("expected connection close after registration test server exits")
	}
	if strings.Contains(err.Error(), "AGENT_ID") {
		t.Fatalf("registration token flow must not require AGENT_ID, got %v", err)
	}
	select {
	case <-helloReceived:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for hello")
	}
	if runtime.config.AgentID != "node_from_registration" {
		t.Fatalf("expected agent id from registration response, got %q", runtime.config.AgentID)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "agent-credential.json"))
	if err != nil {
		t.Fatalf("expected credential file from server hint: %v", err)
	}
	if !strings.Contains(string(data), "long-lived-credential") {
		t.Fatalf("expected persisted credential from server hint, got %s", string(data))
	}
}

func TestNodeRuntimePrefersFinalizedCredentialFileOverInstallRegistrationToken(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"old-credential","registration_finalized":true}`), 0o600); err != nil {
		t.Fatalf("write existing credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime App")
	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "http://127.0.0.1:1",
		"--registration-token", "fresh-registration-token",
		"--credential-file", credentialPath,
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}

	seenAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		seenAuthorization <- request.Header.Get("Authorization")
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "auth_success", map[string]any{
			"agent_id": "node_from_credential",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
	}))
	defer server.Close()
	cfg.ControlPlaneURL = server.URL
	runtime := NewNodeRuntime(cfg, nil)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runtime.runOnce(ctx)
	if err == nil {
		t.Fatalf("expected connection close after test server exits")
	}
	select {
	case authorization := <-seenAuthorization:
		if authorization != "Bearer old-credential" {
			t.Fatalf("expected finalized credential to take precedence, got %q", authorization)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for authorization header")
	}
}

func TestNodeRuntimePrefersInstallRegistrationTokenOverPendingCredentialFile(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"pending-credential","registration_finalized":false}`), 0o600); err != nil {
		t.Fatalf("write pending credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime App")
	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "http://127.0.0.1:1",
		"--registration-token", "retry-registration-token",
		"--credential-file", credentialPath,
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}

	seenAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		seenAuthorization <- request.Header.Get("Authorization")
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_id":         "node_from_registration",
			"agent_credential": "replacement-credential",
		})
		ack := readRuntimeTestEnvelope(t, request.Context(), conn)
		if ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
	}))
	defer server.Close()
	cfg.ControlPlaneURL = server.URL
	runtime := NewNodeRuntime(cfg, nil)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runtime.runOnce(ctx)
	if err == nil {
		t.Fatalf("expected connection close after test server exits")
	}
	select {
	case authorization := <-seenAuthorization:
		if authorization != "Bearer retry-registration-token" {
			t.Fatalf("expected pending credential to keep registration retry path, got %q", authorization)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for authorization header")
	}
	if registrationToken := runtime.getRegistrationToken(); registrationToken != "retry-registration-token" {
		t.Fatalf("expected registration token to remain until finalization, got %q", registrationToken)
	}
	if token := runtime.authToken(); token != "retry-registration-token" {
		t.Fatalf("expected pending credential to keep retry auth token, got %q", token)
	}
}

func TestNodeRuntimeFallsBackToCredentialWhenStoredRegistrationTokenWasFinalized(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"active-credential","registration_finalized":false}`), 0o600); err != nil {
		t.Fatalf("write pending credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime App")
	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "http://127.0.0.1:1",
		"--registration-token", "already-used-registration-token",
		"--credential-file", credentialPath,
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}

	seenAuthorization := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorization := request.Header.Get("Authorization")
		seenAuthorization <- authorization
		if authorization == "Bearer already-used-registration-token" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "auth_success", map[string]any{
			"agent_id": "node_from_credential",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
	}))
	defer server.Close()
	cfg.ControlPlaneURL = server.URL
	runtime := NewNodeRuntime(cfg, nil)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runtime.runOnce(ctx)
	if err == nil {
		t.Fatalf("expected connection close after test server exits")
	}
	first := <-seenAuthorization
	second := <-seenAuthorization
	if first != "Bearer already-used-registration-token" || second != "Bearer active-credential" {
		t.Fatalf("expected registration then credential fallback, got %q then %q", first, second)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read finalized credential file: %v", err)
	}
	if !strings.Contains(string(data), `"registration_finalized":true`) {
		t.Fatalf("expected credential fallback to finalize file, got %s", string(data))
	}
}

func TestNodeRuntimeFallsBackToRegistrationTokenWhenFinalizedCredentialIsRejected(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"stale-credential","registration_finalized":true}`), 0o600); err != nil {
		t.Fatalf("write stale credential file: %v", err)
	}
	t.Setenv("APP_NAME", "Runtime App")
	cfg, err := LoadRuntimeConfigFromArgs([]string{
		"install",
		"--control-url", "http://127.0.0.1:1",
		"--registration-token", "fresh-registration-token",
		"--credential-file", credentialPath,
	})
	if err != nil {
		t.Fatalf("load install config: %v", err)
	}

	seenAuthorization := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorization := request.Header.Get("Authorization")
		seenAuthorization <- authorization
		if authorization == "Bearer stale-credential" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		if authorization != "Bearer fresh-registration-token" {
			t.Errorf("unexpected authorization header %q", authorization)
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_id":         "node_from_registration",
			"agent_credential": "replacement-credential",
		})
		ack := readRuntimeTestEnvelope(t, request.Context(), conn)
		if ack.Type != "registration_ack" {
			t.Errorf("expected registration_ack, got %#v", ack)
			return
		}
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_finalized", map[string]any{
			"agent_id": "node_from_registration",
		})
		hello := readRuntimeTestEnvelope(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
	}))
	defer server.Close()
	cfg.ControlPlaneURL = server.URL
	runtime := NewNodeRuntime(cfg, nil)
	runtime.metricsInterval = time.Hour
	runtime.heartbeatInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runtime.runOnce(ctx)
	if err == nil {
		t.Fatalf("expected connection close after test server exits")
	}
	first := readAuthorizationForRuntimeTest(t, seenAuthorization)
	second := readAuthorizationForRuntimeTest(t, seenAuthorization)
	if first != "Bearer stale-credential" || second != "Bearer fresh-registration-token" {
		t.Fatalf("expected stale credential then registration fallback, got %q then %q", first, second)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read replacement credential file: %v", err)
	}
	if !strings.Contains(string(data), "replacement-credential") {
		t.Fatalf("expected replacement credential file, got %s", string(data))
	}
	if !strings.Contains(string(data), `"registration_finalized":true`) {
		t.Fatalf("expected replacement credential to be finalized, got %s", string(data))
	}
	if registrationToken := runtime.getRegistrationToken(); registrationToken != "" {
		t.Fatalf("expected finalized fallback registration to clear token, got %q", registrationToken)
	}
}

func TestNodeRuntimeFailsRegistrationWhenCredentialCannotBePersisted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		writeRuntimeTestEnvelope(t, request.Context(), conn, "registration_success", map[string]any{
			"agent_credential": "long-lived-credential",
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		ControlPlaneURL:     server.URL,
		AgentID:             "node_1",
		RegistrationToken:   "registration-token",
		AgentCredentialFile: t.TempDir(),
	}, nil)
	if err := runtime.runOnce(ctx); err == nil {
		t.Fatalf("expected credential persistence failure")
	}
	if credential := runtime.getAgentCredential(); credential != "" {
		t.Fatalf("expected unpersisted credential to be discarded, got %q", credential)
	}
	if registrationToken := runtime.getRegistrationToken(); registrationToken != "registration-token" {
		t.Fatalf("expected registration token to remain usable, got %q", registrationToken)
	}
}

func readAuthorizationForRuntimeTest(t *testing.T, seen <-chan string) string {
	t.Helper()
	select {
	case authorization := <-seen:
		return authorization
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for authorization header")
		return ""
	}
}

func TestNodeRuntimeCredentialPersistenceResetsFileMode(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"agent_credential":"old"}`), 0o644); err != nil {
		t.Fatalf("write existing credential file: %v", err)
	}
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Runtime App",
		AgentCredentialFile: credentialPath,
	}, nil)
	if err := runtime.persistCredential("new-secret", true); err != nil {
		t.Fatalf("persist credential: %v", err)
	}
	info, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatalf("stat credential file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected credential file mode 0600, got %o", got)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if !strings.Contains(string(data), "new-secret") {
		t.Fatalf("expected new credential content, got %s", string(data))
	}
}

func TestNodeRuntimeReturnsPermanentConfigurationErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:         "Runtime App",
		ControlPlaneURL: "http://127.0.0.1:1",
		AgentID:         "node_1",
	}, nil)

	err := runtime.Run(ctx)
	if err == nil {
		t.Fatalf("expected static configuration error")
	}
	if !strings.Contains(err.Error(), "agent credential") || !strings.Contains(err.Error(), "enrollment token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordingApplier struct {
	mu      sync.Mutex
	applied ConfigSnapshot
	err     error
}

func (applier *recordingApplier) Apply(ctx context.Context, snapshot ConfigSnapshot) error {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	applier.applied = snapshot
	return applier.err
}

func (applier *recordingApplier) snapshot() ConfigSnapshot {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	return applier.applied
}

type runtimeTestEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func writeRuntimeTestEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn, messageType string, payload any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"type":       messageType,
		"message_id": "test_" + messageType,
		"sent_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"payload":    payload,
	})
	if err != nil {
		t.Fatalf("marshal runtime envelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write runtime envelope: %v", err)
	}
}

func readRuntimeTestEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn) runtimeTestEnvelope {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read runtime envelope: %v", err)
	}
	var envelope runtimeTestEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode runtime envelope %s: %v", strings.TrimSpace(string(data)), err)
	}
	return envelope
}
