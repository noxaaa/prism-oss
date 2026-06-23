package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/service"

	"nhooyr.io/websocket"
)

type agentEnvelope struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	SentAt    string `json:"sent_at"`
	Payload   any    `json:"payload"`
}

type agentIncomingEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (server *ControlServer) handleAgentConnect(response http.ResponseWriter, request *http.Request) {
	if server.controlService == nil {
		writeError(response, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
		return
	}
	token, ok := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	agentType := request.Header.Get("X-Agent-Type")
	if _, err := server.controlService.ValidateAgentToken(request.Context(), agentType, token); err != nil {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}

	conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	authResult, err := server.controlService.AuthenticateAgentToken(request.Context(), agentType, token)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "UNAUTHENTICATED")
		return
	}

	messageType := "auth_success"
	payload := map[string]any{
		"agent_id":   authResult.AgentID,
		"agent_type": authResult.AgentType,
	}
	if authResult.RegisteredWithToken {
		messageType = "registration_success"
		payload["agent_credential"] = authResult.AgentCredential
		payload["agent_credential_file_hint"] = authResult.AgentCredentialFileHint
	}
	if err := writeAgentEnvelope(request.Context(), conn, messageType, payload); err != nil {
		if authResult.RegisteredWithToken {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.controlService.ReleaseAgentRegistrationCredential(ctx, authResult)
		}
		return
	}
	registrationFinalized := !authResult.RegisteredWithToken
	connectionGeneration := int64(0)
	sessionActive := false
	activateSession := func() {
		if sessionActive {
			return
		}
		connectionGeneration = server.agentStates.MarkConnected(authResult.OrganizationID, authResult.AgentType, authResult.AgentID)
		sessionActive = true
	}
	if registrationFinalized {
		activateSession()
	}
	defer func() {
		if authResult.RegisteredWithToken && !registrationFinalized {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.controlService.ReleaseAgentRegistrationCredential(ctx, authResult)
		}
		if !sessionActive {
			return
		}
		if !server.agentStates.MarkDisconnected(authResult.OrganizationID, authResult.AgentType, authResult.AgentID, connectionGeneration) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.markAgentDisconnected(ctx, authResult)
	}()
	server.handleAgentMessages(request.Context(), conn, authResult, observedDirectAgentRemoteAddr(request), &connectionGeneration, &registrationFinalized, activateSession)
}

func observedDirectAgentRemoteAddr(request *http.Request) string {
	if strings.TrimSpace(request.Header.Get("Forwarded")) != "" ||
		strings.TrimSpace(request.Header.Get("X-Forwarded-For")) != "" ||
		strings.TrimSpace(request.Header.Get("X-Real-IP")) != "" {
		return ""
	}
	return request.RemoteAddr
}

func writeAgentEnvelope(ctx context.Context, conn *websocket.Conn, messageType string, payload any) error {
	envelope := agentEnvelope{
		Type:      messageType,
		MessageID: messageType + "_" + time.Now().UTC().Format("20060102150405.000000000"),
		SentAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	}
	return conn.Write(ctx, websocket.MessageText, mustJSON(envelope))
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func (server *ControlServer) handleAgentMessages(ctx context.Context, conn *websocket.Conn, authResult service.AgentAuthResult, remoteAddr string, connectionGeneration *int64, registrationFinalized *bool, activateSession func()) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var envelope agentIncomingEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_MESSAGE"})
			continue
		}
		if !*registrationFinalized {
			if envelope.Type != "registration_ack" {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "REGISTRATION_NOT_FINALIZED"})
				return
			}
		} else if !server.agentStates.IsCurrent(authResult.OrganizationID, authResult.AgentType, authResult.AgentID, *connectionGeneration) {
			return
		}
		switch envelope.Type {
		case "registration_ack":
			if authResult.RegisteredWithToken && !*registrationFinalized {
				var payload struct {
					Status string `json:"status"`
				}
				if err := json.Unmarshal(envelope.Payload, &payload); err != nil || strings.ToUpper(strings.TrimSpace(payload.Status)) != "PERSISTED" {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_REGISTRATION_ACK"})
					return
				}
				if err := server.controlService.FinalizeAgentRegistrationDelivery(ctx, authResult); err != nil {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "REGISTRATION_FINALIZE_FAILED"})
					return
				}
				*registrationFinalized = true
				activateSession()
				if err := writeAgentEnvelope(ctx, conn, "registration_finalized", map[string]any{
					"agent_id":   authResult.AgentID,
					"agent_type": authResult.AgentType,
				}); err != nil {
					return
				}
			}
		case "hello":
			var helloPayload struct {
				AgentVersion   string `json:"agent_version"`
				AgentCommit    string `json:"agent_commit"`
				AgentBuildTime string `json:"agent_build_time"`
			}
			if err := json.Unmarshal(envelope.Payload, &helloPayload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_HELLO"})
				continue
			}
			shouldUpdate := false
			if authResult.AgentType == "NODE" {
				_, update, err := server.controlService.RecordNodeAgentHello(ctx, authResult.OrganizationID, authResult.AgentID, service.AgentHelloInput{Version: helloPayload.AgentVersion, Commit: helloPayload.AgentCommit, BuildTime: helloPayload.AgentBuildTime, RemoteAddr: remoteAddr})
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "STATE_UPDATE_FAILED"})
					continue
				}
				shouldUpdate = update
			} else if err := server.markAgentConnected(ctx, authResult); err != nil {
				if errors.Is(err, service.ErrNotFound) {
					server.closeStaleAgentSession(ctx, conn, authResult)
					return
				}
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "STATE_UPDATE_FAILED"})
				continue
			}
			if authResult.AgentType == "NODE" {
				if shouldUpdate {
					_ = writeAgentEnvelope(ctx, conn, "agent_update_request", agentUpdateRequestPayload(server.controlService.AgentReleaseVersion()))
				}
				config, err := server.controlService.CompileNodeAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "CONFIG_COMPILE_FAILED"})
					continue
				}
				_ = writeAgentEnvelope(ctx, conn, "config_snapshot", config)
			} else if authResult.AgentType == "MONITOR" {
				config, err := server.controlService.CompileMonitorAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "MONITOR_CONFIG_COMPILE_FAILED"})
					continue
				}
				_ = writeAgentEnvelope(ctx, conn, "monitor_config_snapshot", config)
			}
		case "heartbeat":
			if authResult.AgentType == "NODE" {
				var payload struct {
					AppliedConfigVersion int `json:"applied_config_version"`
				}
				if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_HEARTBEAT"})
					continue
				}
				if err := server.controlService.MarkNodeAgentConnected(ctx, authResult.OrganizationID, authResult.AgentID); err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "STATE_UPDATE_FAILED"})
					continue
				}
				if targetVersion, pending, err := server.controlService.PendingNodeAgentUpdate(ctx, authResult.OrganizationID, authResult.AgentID); err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "STATE_UPDATE_FAILED"})
					continue
				} else if pending {
					_ = writeAgentEnvelope(ctx, conn, "agent_update_request", agentUpdateRequestPayload(targetVersion))
				}
				behind, err := server.controlService.NodeAgentConfigBehind(ctx, authResult.OrganizationID, authResult.AgentID, payload.AppliedConfigVersion)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "CONFIG_STATE_FAILED"})
					continue
				}
				if !behind {
					continue
				}
				config, err := server.controlService.CompileNodeAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "CONFIG_COMPILE_FAILED"})
					continue
				}
				_ = writeAgentEnvelope(ctx, conn, "config_snapshot", config)
			} else if authResult.AgentType == "MONITOR" {
				if err := server.markAgentConnected(ctx, authResult); err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "STATE_UPDATE_FAILED"})
					continue
				}
				config, err := server.controlService.CompileMonitorAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "MONITOR_CONFIG_COMPILE_FAILED"})
					continue
				}
				_ = writeAgentEnvelope(ctx, conn, "monitor_config_snapshot", config)
			}
		case "config_ack":
			var payload struct {
				ConfigVersion int                            `json:"config_version"`
				Status        string                         `json:"status"`
				ErrorMessage  string                         `json:"error_message"`
				Errors        []agent.ConfigApplyErrorDetail `json:"errors"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_CONFIG_ACK"})
				continue
			}
			if authResult.AgentType == "NODE" {
				applyErrors := make([]service.ConfigApplyErrorInput, 0, len(payload.Errors))
				for _, applyErr := range payload.Errors {
					applyErrors = append(applyErrors, service.ConfigApplyErrorInput{
						Code:     applyErr.Code,
						RuleIDs:  applyErr.RuleIDs,
						Protocol: applyErr.Protocol,
						ListenIP: applyErr.ListenIP,
						Port:     applyErr.Port,
						Message:  applyErr.Message,
					})
				}
				if err := server.controlService.AcknowledgeNodeAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID, payload.ConfigVersion, payload.Status, payload.ErrorMessage, applyErrors); err != nil {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "CONFIG_ACK_FAILED"})
				}
			}
		case "monitor_config_ack":
			var payload struct {
				ConfigVersion int    `json:"config_version"`
				Status        string `json:"status"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_MONITOR_CONFIG_ACK"})
				continue
			}
			if authResult.AgentType == "MONITOR" && strings.ToUpper(strings.TrimSpace(payload.Status)) == "APPLIED" {
				if err := server.controlService.AcknowledgeMonitorAgentConfig(ctx, authResult.OrganizationID, authResult.AgentID, payload.ConfigVersion); err != nil {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "MONITOR_CONFIG_ACK_FAILED"})
				}
			}
		case "agent_update_result":
			var payload struct {
				Status       string `json:"status"`
				ErrorMessage string `json:"error_message"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_AGENT_UPDATE_RESULT"})
				continue
			}
			if authResult.AgentType == "NODE" {
				if err := server.controlService.RecordNodeAgentUpdateResult(ctx, authResult.OrganizationID, authResult.AgentID, payload.Status, payload.ErrorMessage); err != nil {
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "AGENT_UPDATE_RESULT_FAILED"})
				}
			}
		case "metrics":
			var payload agent.MetricsPayload
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_METRICS"})
				continue
			}
			if !server.agentStates.UpdateMetricsForConnection(authResult.OrganizationID, authResult.AgentType, authResult.AgentID, *connectionGeneration, payload) {
				return
			}
			if authResult.AgentType == "NODE" && payload.TrafficReportID != "" && len(payload.TrafficDeltas) > 0 {
				if _, err := server.controlService.RecordNodeTrafficReport(ctx, authResult.OrganizationID, authResult.AgentID, service.AgentTrafficReportInput{ReportID: payload.TrafficReportID, Deltas: payload.TrafficDeltas}); err != nil {
					if errors.Is(err, service.ErrNotFound) {
						server.closeStaleAgentSession(ctx, conn, authResult)
						return
					}
					_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "TRAFFIC_REPORT_FAILED"})
					continue
				}
			}
			if authResult.AgentType == "NODE" && payload.TrafficReportID != "" && len(payload.TrafficDeltas) > 0 {
				_ = writeAgentEnvelope(ctx, conn, "metrics_ack", map[string]any{"traffic_report_id": payload.TrafficReportID})
			}
		case "health_results":
			var payload struct {
				Results []agent.HealthResultPayload `json:"results"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "INVALID_HEALTH_RESULTS"})
				continue
			}
			if authResult.AgentType != "MONITOR" {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "UNSUPPORTED_AGENT_MESSAGE"})
				continue
			}
			results := make([]service.HealthResultInput, 0, len(payload.Results))
			for _, result := range payload.Results {
				results = append(results, service.HealthResultInput{
					HealthCheckID:       result.HealthCheckID,
					HealthCheckTargetID: result.HealthCheckTargetID,
					TargetID:            result.TargetID,
					Status:              result.Status,
					LatencyMS:           result.LatencyMS,
					ErrorMessage:        result.ErrorMessage,
					ObservedAt:          result.ObservedAt,
				})
			}
			if err := server.controlService.RecordMonitorHealthResults(ctx, authResult.OrganizationID, authResult.AgentID, results); err != nil {
				_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "HEALTH_RESULTS_FAILED"})
				continue
			}
			_ = writeAgentEnvelope(ctx, conn, "health_results_ack", map[string]any{"count": len(results)})
		}
	}
}

func (server *ControlServer) closeStaleAgentSession(ctx context.Context, conn *websocket.Conn, authResult service.AgentAuthResult) {
	if authResult.AgentType == "NODE" {
		_ = writeAgentEnvelope(ctx, conn, "config_snapshot", service.EmptyNodeAgentConfig(authResult.AgentID))
	}
	_ = writeAgentEnvelope(ctx, conn, "error", map[string]any{"code": "AGENT_RECORD_NOT_FOUND"})
	_ = conn.Close(websocket.StatusPolicyViolation, "AGENT_RECORD_NOT_FOUND")
}

func (server *ControlServer) markAgentConnected(ctx context.Context, authResult service.AgentAuthResult) error {
	switch authResult.AgentType {
	case "NODE":
		return server.controlService.MarkNodeAgentConnectedFromRemote(ctx, authResult.OrganizationID, authResult.AgentID, "")
	case "MONITOR":
		return server.controlService.MarkMonitorAgentConnected(ctx, authResult.OrganizationID, authResult.AgentID)
	default:
		return nil
	}
}

func (server *ControlServer) markAgentDisconnected(ctx context.Context, authResult service.AgentAuthResult) error {
	switch authResult.AgentType {
	case "NODE":
		return server.controlService.MarkNodeAgentDisconnected(ctx, authResult.OrganizationID, authResult.AgentID)
	case "MONITOR":
		return server.controlService.MarkMonitorAgentDisconnected(ctx, authResult.OrganizationID, authResult.AgentID)
	default:
		return nil
	}
}

func agentUpdateRequestPayload(targetVersion string) map[string]any {
	releaseBaseURL := "https://github.com/noxaaa/prism-oss/releases/latest/download"
	if targetVersion != "" && targetVersion != "latest" {
		releaseBaseURL = "https://github.com/noxaaa/prism-oss/releases/download/" + url.PathEscape(targetVersion)
	}
	return map[string]any{
		"target_version":   targetVersion,
		"release_base_url": releaseBaseURL,
		"asset_name":       "node-agent-linux",
		"sha256sums_url":   releaseBaseURL + "/SHA256SUMS",
	}
}
