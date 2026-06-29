package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/service"
)

func (server *ControlServer) handleNodeMetricsStream(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	identity := internalIdentityFromClaims(claims, request)
	nodeID := request.PathValue("node_id")
	if err := server.controlService.AuthorizeNodeMetricsStream(request.Context(), identity, nodeID); err != nil {
		writeServiceResponse(response, http.StatusOK, nil, err)
		return
	}
	server.handleAgentMetricsStream(response, request, identity.OrganizationID, "NODE", nodeID, true)
}

func (server *ControlServer) handleNodeMetricsStreamByOrganization(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	identity := internalIdentityFromClaims(claims, request)
	if err := server.controlService.AuthorizeOrganizationNodeMetricsStream(request.Context(), identity); err != nil {
		writeServiceResponse(response, http.StatusOK, nil, err)
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Connection", "keep-alive")
	lastSent := map[string]string{}
	visibleIDs, err := server.visibleNodeIDs(request.Context(), identity)
	if err != nil {
		writeServiceResponse(response, http.StatusOK, nil, err)
		return
	}
	server.writeVisibleNodeMetrics(response, identity.OrganizationID, visibleIDs, lastSent)
	if len(lastSent) == 0 {
		if !writeKeepaliveSSE(response) {
			return
		}
	}
	flushSSE(response)
	if request.URL.Query().Get("once") == "true" {
		return
	}
	metricsTicker := time.NewTicker(time.Second)
	visibilityTicker := time.NewTicker(30 * time.Second)
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer metricsTicker.Stop()
	defer visibilityTicker.Stop()
	defer keepaliveTicker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-visibilityTicker.C:
			nextVisibleIDs, err := server.visibleNodeIDs(request.Context(), identity)
			if err == nil {
				visibleIDs = nextVisibleIDs
			}
		case <-metricsTicker.C:
			if server.writeVisibleNodeMetrics(response, identity.OrganizationID, visibleIDs, lastSent) {
				flushSSE(response)
			}
		case <-keepaliveTicker.C:
			if !writeKeepaliveSSE(response) {
				return
			}
			flushSSE(response)
		}
	}
}

func (server *ControlServer) handleMonitorMetricsStream(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	identity := internalIdentityFromClaims(claims, request)
	monitorID := request.PathValue("monitor_id")
	if _, err := server.controlService.GetMonitor(request.Context(), identity, monitorID); err != nil {
		writeServiceResponse(response, http.StatusOK, nil, err)
		return
	}
	server.handleAgentMetricsStream(response, request, identity.OrganizationID, "MONITOR", monitorID, false)
}

func (server *ControlServer) visibleNodeIDs(ctx context.Context, identity service.InternalIdentity) (map[string]bool, error) {
	nodes, err := server.controlService.ListNodes(ctx, identity)
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		visible[node.ID] = true
	}
	return visible, nil
}

func (server *ControlServer) writeVisibleNodeMetrics(response http.ResponseWriter, organizationID string, visibleIDs map[string]bool, lastSent map[string]string) bool {
	wrote := false
	for _, state := range server.agentStates.LatestByOrganizationAndType(organizationID, "NODE") {
		if !visibleIDs[state.AgentID] || state.LastSeenAt == lastSent[state.AgentID] {
			continue
		}
		if !writeNodeMetricsSSE(response, state) {
			return false
		}
		lastSent[state.AgentID] = state.LastSeenAt
		wrote = true
	}
	return wrote
}

func (server *ControlServer) handleAgentMetricsStream(response http.ResponseWriter, request *http.Request, organizationID string, agentType string, agentID string, includeHostDetails bool) {
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Connection", "keep-alive")
	lastSent := ""
	if state, ok := server.agentStates.Latest(organizationID, agentType, agentID); ok {
		if !writeMetricsSSE(response, state, includeHostDetails) {
			return
		}
		lastSent = state.LastSeenAt
	} else {
		if !writeKeepaliveSSE(response) {
			return
		}
	}
	flushSSE(response)
	if request.URL.Query().Get("once") == "true" {
		return
	}
	metricsTicker := time.NewTicker(time.Second)
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer metricsTicker.Stop()
	defer keepaliveTicker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-metricsTicker.C:
			state, ok := server.agentStates.Latest(organizationID, agentType, agentID)
			if !ok || state.LastSeenAt == lastSent {
				continue
			}
			if !writeMetricsSSE(response, state, includeHostDetails) {
				return
			}
			lastSent = state.LastSeenAt
			flushSSE(response)
		case <-keepaliveTicker.C:
			if !writeKeepaliveSSE(response) {
				return
			}
			flushSSE(response)
		}
	}
}

func writeKeepaliveSSE(response http.ResponseWriter) bool {
	_, err := fmt.Fprint(response, "event: keepalive\ndata: {}\n\n")
	return err == nil
}

func writeMetricsSSE(response http.ResponseWriter, state AgentMetricsState, includeHostDetails bool) bool {
	data, _ := json.Marshal(agentMetricsPayload(state, includeHostDetails))
	_, err := fmt.Fprintf(response, "event: metrics\ndata: %s\n\n", data)
	return err == nil
}

func writeNodeMetricsSSE(response http.ResponseWriter, state AgentMetricsState) bool {
	payload := map[string]any{
		"node_id": state.AgentID,
		"metrics": agentMetricsPayload(state, true),
	}
	data, _ := json.Marshal(payload)
	_, err := fmt.Fprintf(response, "event: metrics\ndata: %s\n\n", data)
	return err == nil
}

func agentMetricsPayload(state AgentMetricsState, includeHostDetails bool) map[string]any {
	payload := map[string]any{
		"status":                 state.Status,
		"last_seen_at":           state.LastSeenAt,
		"tcp_connections":        state.Metrics.TCPConnections,
		"udp_packets_per_second": state.Metrics.UDPPacketsPerSecond,
		"bandwidth_bps":          state.Metrics.BandwidthBps,
		"cpu_percent":            state.Metrics.CPUPercent,
		"ram_used_bytes":         state.Metrics.RAMUsedBytes,
		"ram_total_bytes":        state.Metrics.RAMTotalBytes,
		"disk_used_bytes":        state.Metrics.DiskUsedBytes,
		"disk_total_bytes":       state.Metrics.DiskTotalBytes,
		"upload_bytes":           state.Metrics.UploadBytes,
		"download_bytes":         state.Metrics.DownloadBytes,
		"uptime_seconds":         state.Metrics.UptimeSeconds,
		"boot_time":              state.Metrics.BootTime,
		"applied_config_version": state.Metrics.AppliedConfigVersion,
		"targets":                state.Metrics.Targets,
	}
	if includeHostDetails {
		payload["cpu_model"] = state.Metrics.CPUModel
		payload["cpu_logical_cores"] = state.Metrics.CPULogicalCores
		payload["cpu_physical_cores"] = state.Metrics.CPUPhysicalCores
		payload["os_name"] = state.Metrics.OSName
		payload["os_version"] = state.Metrics.OSVersion
		payload["kernel_version"] = state.Metrics.KernelVersion
		payload["architecture"] = state.Metrics.Architecture
		payload["virtualization_system"] = state.Metrics.VirtualizationSystem
		payload["virtualization_role"] = state.Metrics.VirtualizationRole
	}
	return payload
}

func flushSSE(response http.ResponseWriter) {
	if flusher, ok := response.(http.Flusher); ok {
		flusher.Flush()
	}
}
