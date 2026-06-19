package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/core/validator"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

type ControlServerOptions struct {
	TokenVerifier           auth.InternalTokenVerifier
	WebUserVerifier         auth.WebUserTokenVerifier
	RepositoryStore         repo.UnitOfWork
	ControlService          *service.ControlService
	InternalTokenTTL        time.Duration
	AppName                 string
	ControlPlaneURL         string
	AgentReleaseVersion     string
	AgentTokenSigningSecret []byte
	AgentStateRegistry      *AgentStateRegistry
	Edition                 edition.Provider
	RouteExtensions         []ControlRouteExtension
}

type ControlServer struct {
	tokenVerifier   auth.InternalTokenVerifier
	webUserVerifier auth.WebUserTokenVerifier
	controlService  *service.ControlService
	agentStates     *AgentStateRegistry
	edition         edition.Provider
	routeExtensions []ControlRouteExtension
	mux             *http.ServeMux
}

func NewControlServer(options ControlServerOptions) *ControlServer {
	provider := options.Edition
	if provider == nil {
		provider = defaultControlServerEdition()
	}
	controlService := options.ControlService
	if controlService == nil && options.RepositoryStore != nil {
		controlService = service.NewControlServiceWithOptions(options.RepositoryStore, service.ControlServiceOptions{
			AppName:                 options.AppName,
			ControlPlaneURL:         options.ControlPlaneURL,
			AgentReleaseVersion:     options.AgentReleaseVersion,
			AgentTokenSigningSecret: options.AgentTokenSigningSecret,
			Edition:                 provider,
		})
	}
	server := &ControlServer{
		tokenVerifier:   options.TokenVerifier,
		webUserVerifier: options.WebUserVerifier,
		controlService:  controlService,
		agentStates:     options.AgentStateRegistry,
		edition:         provider,
		routeExtensions: append([]ControlRouteExtension(nil), options.RouteExtensions...),
		mux:             http.NewServeMux(),
	}
	if server.agentStates == nil {
		server.agentStates = NewAgentStateRegistry()
	}
	server.routes()
	return server
}

func (server *ControlServer) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	server.mux.ServeHTTP(response, request)
}

func (server *ControlServer) routes() {
	server.mux.HandleFunc("GET /healthz", func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, http.StatusOK, map[string]any{"status": "ok"})
	})
	server.mux.HandleFunc("GET /agent/v1/connect", server.handleAgentConnect)
	server.mux.HandleFunc("GET /internal/v1/organizations/current", server.withInternalIdentity(func(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
		if server.controlService == nil {
			if !hasClaimPermission(claims.Permissions, string(domain.PermissionOrganizationRead)) {
				writeError(response, http.StatusForbidden, "FORBIDDEN")
				return
			}
			writeJSON(response, http.StatusOK, map[string]any{"data": map[string]any{"organization_id": claims.OrganizationID, "member_id": claims.MemberID}})
			return
		}
		session, err := server.controlService.SessionForInternalIdentity(request.Context(), internalIdentityFromClaims(claims, request))
		writeServiceResponse(response, http.StatusOK, session.Organization, err)
	}))
	server.mux.HandleFunc("PATCH /internal/v1/organizations/current", server.withInternalIdentity(server.handleUpdateOrganization))
	server.mux.HandleFunc("POST /internal/v1/bootstrap", server.withWebUser(auth.WebUserTokenPurposeBootstrap, server.handleBootstrap))
	server.mux.HandleFunc("GET /internal/v1/session", server.withWebUser(auth.WebUserTokenPurposeSession, server.handleSession))
	if server.edition.Has(edition.CapabilityRBAC) {
		server.routesRBAC()
	}
	for _, extension := range server.routeExtensions {
		extension.RegisterControlRoutes(controlRouteRegistry{server: server})
	}
	server.mux.HandleFunc("GET /internal/v1/resource-options/node-groups", server.withInternalIdentity(server.handleNodeGroupOptions))
	server.mux.HandleFunc("GET /internal/v1/resource-options/node-group-listen-ips", server.withInternalIdentity(server.handleNodeGroupListenIPOptions))
	server.mux.HandleFunc("GET /internal/v1/resource-options/targets", server.withInternalIdentity(server.handleTargetOptions))
	server.mux.HandleFunc("GET /internal/v1/resource-options/target-groups", server.withInternalIdentity(server.handleTargetGroupOptions))
	server.mux.HandleFunc("GET /internal/v1/node-groups", server.withInternalIdentity(server.handleListNodeGroups))
	server.mux.HandleFunc("POST /internal/v1/node-groups", server.withInternalIdentity(server.handleCreateNodeGroup))
	server.mux.HandleFunc("PATCH /internal/v1/node-groups/{group_id}", server.withInternalIdentity(server.handleUpdateNodeGroup))
	server.mux.HandleFunc("DELETE /internal/v1/node-groups/{group_id}", server.withInternalIdentity(server.handleDeleteNodeGroup))
	server.mux.HandleFunc("GET /internal/v1/nodes", server.withInternalIdentity(server.handleListNodes))
	server.mux.HandleFunc("POST /internal/v1/nodes/agent-upgrade", server.withInternalIdentity(server.handleRequestNodeAgentUpgrades))
	server.mux.HandleFunc("POST /internal/v1/nodes", server.withInternalIdentity(server.handleCreateNode))
	server.mux.HandleFunc("GET /internal/v1/nodes/{node_id}", server.withInternalIdentity(server.handleGetNode))
	server.mux.HandleFunc("GET /internal/v1/nodes/{node_id}/metrics/stream", server.withInternalIdentity(server.handleNodeMetricsStream))
	server.mux.HandleFunc("PATCH /internal/v1/nodes/{node_id}", server.withInternalIdentity(server.handleUpdateNode))
	server.mux.HandleFunc("PATCH /internal/v1/nodes/{node_id}/agent-update-policy", server.withInternalIdentity(server.handleUpdateNodeAgentUpdatePolicy))
	server.mux.HandleFunc("POST /internal/v1/nodes/{node_id}/agent-upgrade", server.withInternalIdentity(server.handleRequestNodeAgentUpgrade))
	server.mux.HandleFunc("DELETE /internal/v1/nodes/{node_id}", server.withInternalIdentity(server.handleDeleteNode))
	server.mux.HandleFunc("GET /internal/v1/nodes/{node_id}/registration-tokens", server.withInternalIdentity(server.handleListNodeRegistrationTokens))
	server.mux.HandleFunc("POST /internal/v1/nodes/{node_id}/registration-token", server.withInternalIdentity(server.handleCreateNodeRegistrationToken))
	server.mux.HandleFunc("DELETE /internal/v1/nodes/{node_id}/registration-tokens/{token_id}", server.withInternalIdentity(server.handleRevokeNodeRegistrationToken))
	if server.edition.Has(edition.CapabilityMonitors) {
		server.mux.HandleFunc("GET /internal/v1/monitor-groups", server.withInternalIdentity(server.handleListMonitorGroups))
		server.mux.HandleFunc("POST /internal/v1/monitor-groups", server.withInternalIdentity(server.handleCreateMonitorGroup))
		server.mux.HandleFunc("PATCH /internal/v1/monitor-groups/{group_id}", server.withInternalIdentity(server.handleUpdateMonitorGroup))
		server.mux.HandleFunc("DELETE /internal/v1/monitor-groups/{group_id}", server.withInternalIdentity(server.handleDeleteMonitorGroup))
		server.mux.HandleFunc("GET /internal/v1/monitors", server.withInternalIdentity(server.handleListMonitors))
		server.mux.HandleFunc("POST /internal/v1/monitors", server.withInternalIdentity(server.handleCreateMonitor))
		server.mux.HandleFunc("GET /internal/v1/monitors/{monitor_id}", server.withInternalIdentity(server.handleGetMonitor))
		server.mux.HandleFunc("GET /internal/v1/monitors/{monitor_id}/metrics/stream", server.withInternalIdentity(server.handleMonitorMetricsStream))
		server.mux.HandleFunc("PATCH /internal/v1/monitors/{monitor_id}", server.withInternalIdentity(server.handleUpdateMonitor))
		server.mux.HandleFunc("DELETE /internal/v1/monitors/{monitor_id}", server.withInternalIdentity(server.handleDeleteMonitor))
		server.mux.HandleFunc("GET /internal/v1/monitors/{monitor_id}/registration-tokens", server.withInternalIdentity(server.handleListMonitorRegistrationTokens))
		server.mux.HandleFunc("POST /internal/v1/monitors/{monitor_id}/registration-token", server.withInternalIdentity(server.handleCreateMonitorRegistrationToken))
		server.mux.HandleFunc("DELETE /internal/v1/monitors/{monitor_id}/registration-tokens/{token_id}", server.withInternalIdentity(server.handleRevokeMonitorRegistrationToken))
	}
	server.mux.HandleFunc("GET /internal/v1/targets", server.withInternalIdentity(server.handleListTargets))
	server.mux.HandleFunc("POST /internal/v1/targets", server.withInternalIdentity(server.handleCreateTarget))
	server.mux.HandleFunc("PATCH /internal/v1/targets/{target_id}", server.withInternalIdentity(server.handleUpdateTarget))
	server.mux.HandleFunc("DELETE /internal/v1/targets/{target_id}", server.withInternalIdentity(server.handleDeleteTarget))
	server.mux.HandleFunc("GET /internal/v1/target-groups", server.withInternalIdentity(server.handleListTargetGroups))
	server.mux.HandleFunc("POST /internal/v1/target-groups", server.withInternalIdentity(server.handleCreateTargetGroup))
	server.mux.HandleFunc("PATCH /internal/v1/target-groups/{target_group_id}", server.withInternalIdentity(server.handleUpdateTargetGroup))
	server.mux.HandleFunc("DELETE /internal/v1/target-groups/{target_group_id}", server.withInternalIdentity(server.handleDeleteTargetGroup))
	server.mux.HandleFunc("GET /internal/v1/rules", server.withInternalIdentity(server.handleListRules))
	server.mux.HandleFunc("POST /internal/v1/rules", server.withInternalIdentity(server.handleCreateRule))
	server.mux.HandleFunc("GET /internal/v1/rules/export", server.withInternalIdentity(server.handleExportRules))
	server.mux.HandleFunc("POST /internal/v1/rules/import", server.withInternalIdentity(server.handleImportRules))
	server.mux.HandleFunc("POST /internal/v1/rules/batch", server.withInternalIdentity(server.handleBatchRules))
	server.mux.HandleFunc("GET /internal/v1/rules/{rule_id}", server.withInternalIdentity(server.handleGetRule))
	server.mux.HandleFunc("PATCH /internal/v1/rules/{rule_id}", server.withInternalIdentity(server.handleUpdateRule))
	server.mux.HandleFunc("DELETE /internal/v1/rules/{rule_id}", server.withInternalIdentity(server.handleDeleteRule))
	server.mux.HandleFunc("POST /internal/v1/rules/{rule_id}/enable", server.withInternalIdentity(server.handleEnableRule))
	server.mux.HandleFunc("POST /internal/v1/rules/{rule_id}/disable", server.withInternalIdentity(server.handleDisableRule))
	server.mux.HandleFunc("POST /internal/v1/rules/{rule_id}/copy", server.withInternalIdentity(server.handleCopyRule))
	server.mux.HandleFunc("GET /internal/v1/rules/{rule_id}/traffic", server.withInternalIdentity(server.handleRuleTraffic))
	server.mux.HandleFunc("GET /internal/v1/rules/{rule_id}/diagnostics", server.withInternalIdentity(server.handleRuleDiagnostics))
}

func (server *ControlServer) withInternalIdentity(next func(http.ResponseWriter, *http.Request, auth.InternalClaims)) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		header := request.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		claims, err := server.tokenVerifier.Verify(token)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		next(response, request, claims)
	}
}

func (server *ControlServer) withWebUser(purpose auth.WebUserTokenPurpose, next func(http.ResponseWriter, *http.Request, auth.WebUserClaims)) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if server.webUserVerifier == nil {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		header := request.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		claims, err := server.webUserVerifier.Verify(token, purpose)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		next(response, request, claims)
	}
}

func (server *ControlServer) handleBootstrap(response http.ResponseWriter, request *http.Request, claims auth.WebUserClaims) {
	var raw validator.BootstrapRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	input, err := validator.ValidateBootstrapRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	result, err := server.controlService.Bootstrap(request.Context(), webIdentityFromClaims(claims), service.BootstrapInput{
		OrganizationName: input.OrganizationName,
		OrganizationSlug: input.OrganizationSlug,
		SourceIP:         request.RemoteAddr,
	})
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeServiceResponse(response, status, result, err)
}

func (server *ControlServer) handleSession(response http.ResponseWriter, request *http.Request, claims auth.WebUserClaims) {
	result, err := server.controlService.SessionForWebUser(request.Context(), webIdentityFromClaims(claims))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleUpdateOrganization(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var raw validator.OrganizationRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	input, err := validator.ValidateOrganizationRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	result, err := server.controlService.UpdateOrganization(request.Context(), internalIdentityFromClaims(claims, request), service.OrganizationUpdateInput{Name: input.Name, Slug: input.Slug})
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleNodeGroupOptions(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	access := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("access")))
	if access == "" {
		access = "USE"
	}
	result, err := server.controlService.ListNodeGroupOptions(request.Context(), internalIdentityFromClaims(claims, request), access)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleNodeGroupListenIPOptions(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	nodeGroupID := strings.TrimSpace(request.URL.Query().Get("node_group_id"))
	protocol := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("protocol")))
	portText := strings.TrimSpace(request.URL.Query().Get("port"))
	port := 0
	if portText != "" {
		parsedPort, err := strconv.Atoi(portText)
		if err != nil {
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return
		}
		port = parsedPort
	}
	if nodeGroupID == "" || ((protocol == "") != (port == 0)) || (protocol != "" && protocol != "TCP" && protocol != "UDP" && protocol != "TCP_UDP") || port < 0 || port > 65535 {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	result, err := server.controlService.NodeGroupListenIPOptions(request.Context(), internalIdentityFromClaims(claims, request), nodeGroupID, protocol, port)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleTargetOptions(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.TargetOptions(request.Context(), internalIdentityFromClaims(claims, request), "")
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleTargetGroupOptions(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.TargetGroupOptions(request.Context(), internalIdentityFromClaims(claims, request), "")
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleListNodeGroups(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListNodeGroups(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateNodeGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateNodeGroup(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateNodeGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateNodeGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("group_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteNodeGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteNodeGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("group_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListNodes(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListNodes(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateNode(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeNodeInput(response, request, false)
	if !ok {
		return
	}
	result, err := server.controlService.CreateNode(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleGetNode(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.GetNode(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("node_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleUpdateNode(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeNodeInput(response, request, true)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateNode(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("node_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleUpdateNodeAgentUpdatePolicy(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var input service.AgentUpdatePolicyInput
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeJSONDecodeError(response, err)
		return
	}
	result, err := server.controlService.UpdateNodeAgentUpdatePolicy(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("node_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleRequestNodeAgentUpgrade(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.RequestNodeAgentUpgrade(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("node_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleRequestNodeAgentUpgrades(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var input service.AgentUpgradeBatchInput
	if request.Body != nil {
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeJSONDecodeError(response, err)
			return
		}
	}
	result, err := server.controlService.RequestNodeAgentUpgrades(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteNode(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteNode(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("node_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListNodeRegistrationTokens(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListRegistrationTokens(request.Context(), internalIdentityFromClaims(claims, request), "NODE", request.PathValue("node_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateNodeRegistrationToken(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeRegistrationTokenInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateRegistrationToken(request.Context(), internalIdentityFromClaims(claims, request), "NODE", request.PathValue("node_id"), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleRevokeNodeRegistrationToken(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.RevokeRegistrationToken(request.Context(), internalIdentityFromClaims(claims, request), "NODE", request.PathValue("node_id"), request.PathValue("token_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"revoked": true}, err)
}

func (server *ControlServer) handleListMonitorGroups(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListMonitorGroups(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateMonitorGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateMonitorGroup(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateMonitorGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateMonitorGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("group_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteMonitorGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteMonitorGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("group_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListMonitors(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListMonitors(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateMonitor(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeMonitorInput(response, request, false)
	if !ok {
		return
	}
	result, err := server.controlService.CreateMonitor(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleGetMonitor(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.GetMonitor(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("monitor_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleUpdateMonitor(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeMonitorInput(response, request, true)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateMonitor(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("monitor_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteMonitor(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteMonitor(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("monitor_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListMonitorRegistrationTokens(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListRegistrationTokens(request.Context(), internalIdentityFromClaims(claims, request), "MONITOR", request.PathValue("monitor_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateMonitorRegistrationToken(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeRegistrationTokenInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateRegistrationToken(request.Context(), internalIdentityFromClaims(claims, request), "MONITOR", request.PathValue("monitor_id"), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleRevokeMonitorRegistrationToken(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.RevokeRegistrationToken(request.Context(), internalIdentityFromClaims(claims, request), "MONITOR", request.PathValue("monitor_id"), request.PathValue("token_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"revoked": true}, err)
}

func (server *ControlServer) handleListTargets(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListTargets(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateTarget(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeTargetInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateTarget(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateTarget(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeTargetInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateTarget(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("target_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteTarget(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteTarget(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("target_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListTargetGroups(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListTargetGroups(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateTargetGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeTargetGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateTargetGroup(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateTargetGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeTargetGroupInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateTargetGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("target_group_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteTargetGroup(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteTargetGroup(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("target_group_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func decodeGroupInput(response http.ResponseWriter, request *http.Request) (service.GroupMutationInput, bool) {
	var raw validator.GroupRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.GroupMutationInput{}, false
	}
	normalized, err := validator.ValidateGroupRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.GroupMutationInput{}, false
	}
	return service.GroupMutationInput{Name: normalized.Name, Description: normalized.Description}, true
}

func decodeNodeInput(response http.ResponseWriter, request *http.Request, update bool) (service.NodeMutationInput, bool) {
	if update {
		var raw validator.NodePatchRequest
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return service.NodeMutationInput{}, false
		}
		normalized, err := validator.ValidateNodePatchRequest(raw)
		if err != nil {
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return service.NodeMutationInput{}, false
		}
		input := service.NodeMutationInput{}
		if normalized.Name != nil {
			input.Name = *normalized.Name
			input.NameProvided = true
		}
		if normalized.GroupIDs != nil {
			input.GroupIDs = *normalized.GroupIDs
			input.GroupIDsProvided = true
		}
		if normalized.ListenIPs != nil {
			input.ListenIPs = toServiceListenIPs(*normalized.ListenIPs)
			input.ListenIPsProvided = true
		}
		if normalized.PortRanges != nil {
			input.PortRanges = toServicePortRanges(*normalized.PortRanges)
			input.PortRangesProvided = true
		}
		if normalized.PublicDescription != nil {
			input.PublicDescription = *normalized.PublicDescription
			input.PublicDescriptionProvided = true
		}
		return input, true
	}

	var raw validator.NodeRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.NodeMutationInput{}, false
	}
	normalized, err := validator.ValidateNodeRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.NodeMutationInput{}, false
	}
	return service.NodeMutationInput{
		Name:                      normalized.Name,
		NameProvided:              true,
		GroupIDs:                  normalized.GroupIDs,
		GroupIDsProvided:          true,
		ListenIPs:                 toServiceListenIPs(normalized.ListenIPs),
		ListenIPsProvided:         true,
		PortRanges:                toServicePortRanges(normalized.PortRanges),
		PortRangesProvided:        true,
		PublicDescription:         normalized.PublicDescription,
		PublicDescriptionProvided: true,
	}, true
}

func decodeMonitorInput(response http.ResponseWriter, request *http.Request, update bool) (service.MonitorMutationInput, bool) {
	if update {
		var raw validator.MonitorPatchRequest
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return service.MonitorMutationInput{}, false
		}
		normalized, err := validator.ValidateMonitorPatchRequest(raw)
		if err != nil {
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return service.MonitorMutationInput{}, false
		}
		input := service.MonitorMutationInput{}
		if normalized.Name != nil {
			input.Name = *normalized.Name
			input.NameProvided = true
		}
		if normalized.GroupIDs != nil {
			input.GroupIDs = *normalized.GroupIDs
			input.GroupIDsProvided = true
		}
		return input, true
	}

	var raw validator.MonitorRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.MonitorMutationInput{}, false
	}
	normalized, err := validator.ValidateMonitorRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.MonitorMutationInput{}, false
	}
	return service.MonitorMutationInput{Name: normalized.Name, NameProvided: true, GroupIDs: normalized.GroupIDs, GroupIDsProvided: true}, true
}

func decodeRegistrationTokenInput(response http.ResponseWriter, request *http.Request) (service.RegistrationTokenInput, bool) {
	var raw validator.RegistrationTokenRequest
	if request.Body != nil {
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return service.RegistrationTokenInput{}, true
			}
			writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
			return service.RegistrationTokenInput{}, false
		}
	}
	normalized, err := validator.ValidateRegistrationTokenRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.RegistrationTokenInput{}, false
	}
	return service.RegistrationTokenInput{TTLHours: normalized.TTLHours}, true
}

func decodeTargetInput(response http.ResponseWriter, request *http.Request) (service.TargetMutationInput, bool) {
	var raw validator.TargetRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.TargetMutationInput{}, false
	}
	normalized, err := validator.ValidateTargetRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.TargetMutationInput{}, false
	}
	input := service.TargetMutationInput{Name: normalized.Name, Host: normalized.Host, Port: normalized.Port, Enabled: normalized.Enabled}
	if normalized.TargetGroupIDs != nil {
		input.TargetGroupIDs = *normalized.TargetGroupIDs
		input.TargetGroupIDsProvided = true
	}
	return input, true
}

func decodeTargetGroupInput(response http.ResponseWriter, request *http.Request) (service.TargetGroupMutationInput, bool) {
	var raw validator.TargetGroupRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.TargetGroupMutationInput{}, false
	}
	normalized, err := validator.ValidateTargetGroupRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.TargetGroupMutationInput{}, false
	}
	members := make([]service.TargetGroupMemberInput, 0, len(normalized.Members))
	for _, member := range normalized.Members {
		members = append(members, service.TargetGroupMemberInput{TargetID: member.TargetID, Priority: member.Priority, Enabled: member.Enabled})
	}
	return service.TargetGroupMutationInput{Name: normalized.Name, Description: normalized.Description, Members: members}, true
}

func toServiceListenIPs(values []validator.NodeListenIP) []service.NodeListenIPInput {
	inputs := make([]service.NodeListenIPInput, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, service.NodeListenIPInput{ListenIP: value.ListenIP, DisplayName: value.DisplayName})
	}
	return inputs
}

func toServicePortRanges(values []validator.NodePortRange) []service.NodePortRangeInput {
	inputs := make([]service.NodePortRangeInput, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, service.NodePortRangeInput{Protocol: value.Protocol, StartPort: value.StartPort, EndPort: value.EndPort})
	}
	return inputs
}

func webIdentityFromClaims(claims auth.WebUserClaims) service.WebUserIdentity {
	return service.WebUserIdentity{UserID: claims.UserID, Email: claims.Email, Name: claims.Name}
}

func internalIdentityFromClaims(claims auth.InternalClaims, request *http.Request) service.InternalIdentity {
	scopes := make([]service.ResourceScopePayload, 0, len(claims.ResourceScopes))
	for _, scope := range claims.ResourceScopes {
		scopes = append(scopes, service.ResourceScopePayload{ResourceType: scope.ResourceType, ResourceID: scope.ResourceID, AccessLevel: scope.AccessLevel})
	}
	return service.InternalIdentity{
		UserID:         claims.UserID,
		OrganizationID: claims.OrganizationID,
		MemberID:       claims.MemberID,
		Roles:          claims.Roles,
		Permissions:    claims.Permissions,
		ResourceScopes: scopes,
		SourceIP:       request.RemoteAddr,
	}
}

func writeServiceResponse(response http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		var domainErr *domain.DomainError
		switch {
		case errors.As(err, &domainErr) && (domainErr.Code == domain.ErrRulePortConflict || domainErr.Code == domain.ErrRuleDuplicateSNI):
			writeErrorPayload(response, http.StatusConflict, service.ErrorPayloadForError(err))
		case errors.Is(err, service.ErrForbidden):
			writeErrorPayload(response, http.StatusForbidden, service.ErrorPayloadForError(err))
		case errors.Is(err, service.ErrNotFound):
			writeErrorPayload(response, http.StatusNotFound, service.ErrorPayloadForError(err))
		case errors.Is(err, service.ErrConflict):
			writeErrorPayload(response, http.StatusConflict, service.ErrorPayloadForError(err))
		case errors.Is(err, service.ErrQuotaExceeded):
			writeErrorPayload(response, http.StatusConflict, service.ErrorPayloadForError(err))
		case errors.Is(err, service.ErrInvalidInput):
			writeErrorPayload(response, http.StatusBadRequest, service.ErrorPayloadForError(err))
		default:
			writeErrorPayload(response, http.StatusInternalServerError, service.ErrorPayloadForError(err))
		}
		return
	}
	writeJSON(response, status, map[string]any{"data": value})
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeErrorPayload(response, status, defaultErrorPayload(code))
}

func writeValidationError(response http.ResponseWriter, status int, message string, details map[string]any) {
	writeErrorPayload(response, status, service.ErrorPayload{Code: "VALIDATION_FAILED", Message: message, Details: details})
}

func writeJSONDecodeError(response http.ResponseWriter, err error) {
	writeValidationError(response, http.StatusBadRequest, "Request body must be valid JSON.", map[string]any{
		"error": err.Error(),
	})
}

func writeValidatorError(response http.ResponseWriter, err error) {
	var validationErr *validator.ValidationError
	if errors.As(err, &validationErr) {
		writeValidationError(response, http.StatusBadRequest, validationErr.Message, validationErr.Details)
		return
	}
	writeValidationError(response, http.StatusBadRequest, "The request payload is invalid.", map[string]any{
		"error": err.Error(),
	})
}

func writeErrorPayload(response http.ResponseWriter, status int, payload service.ErrorPayload) {
	writeJSON(response, status, map[string]any{"error": payload})
}

func defaultErrorPayload(code string) service.ErrorPayload {
	switch code {
	case "UNAUTHENTICATED":
		return service.ErrorPayload{Code: code, Message: "Authentication is required."}
	case "FORBIDDEN":
		return service.ErrorPayload{Code: code, Message: "You do not have permission to perform this action."}
	case "NOT_FOUND":
		return service.ErrorPayload{Code: code, Message: "The requested resource was not found."}
	case "CONFLICT":
		return service.ErrorPayload{Code: code, Message: "The request conflicts with the current environment state."}
	case "QUOTA_EXCEEDED":
		return service.ErrorPayload{Code: code, Message: "The request exceeds the configured quota."}
	case "VALIDATION_FAILED":
		return service.ErrorPayload{Code: code, Message: "The request payload is invalid."}
	default:
		return service.ErrorPayload{Code: code, Message: "An internal error occurred."}
	}
}

func hasClaimPermission(permissions []string, expected string) bool {
	for _, permission := range permissions {
		if permission == expected {
			return true
		}
	}
	return false
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
