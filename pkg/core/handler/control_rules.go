package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/core/validator"
)

func (server *ControlServer) handleListRules(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListRules(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeRuleInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateRule(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleGetRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.GetRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleUpdateRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeRuleInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleEnableRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.EnableRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDisableRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.DisableRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCopyRule(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeRuleCopyInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CopyRule(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleRuleTraffic(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.RuleTraffic(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("rule_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleRuleDiagnostics(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	identity := internalIdentityFromClaims(claims, request)
	states := server.agentStates.LatestByOrganizationAndType(identity.OrganizationID, "NODE")
	runtimeStates := make([]service.AgentRuntimeMetricsInput, 0, len(states))
	for _, state := range states {
		runtimeStates = append(runtimeStates, service.AgentRuntimeMetricsInput{
			AgentID:    state.AgentID,
			Status:     state.Status,
			LastSeenAt: state.LastSeenAt,
			Metrics:    state.Metrics,
		})
	}
	result, err := server.controlService.RuleDiagnostics(request.Context(), identity, request.PathValue("rule_id"), runtimeStates)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleExportRules(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var ruleIDs []string
	if rawRuleIDs := strings.TrimSpace(request.URL.Query().Get("rule_ids")); rawRuleIDs != "" {
		for _, ruleID := range strings.Split(rawRuleIDs, ",") {
			ruleID = strings.TrimSpace(ruleID)
			if ruleID != "" {
				ruleIDs = append(ruleIDs, ruleID)
			}
		}
	}
	result, err := server.controlService.ExportRules(request.Context(), internalIdentityFromClaims(claims, request), ruleIDs)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleImportRules(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var raw struct {
		Entry      service.RuleImportEntry `json:"entry"`
		Format     string                  `json:"format"`
		SourceText string                  `json:"source_text"`
	}
	if request.Body != nil {
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			writeJSONDecodeError(response, err)
			return
		}
	}
	importOptions := validator.RuleImportRequest{}
	if queryDryRun := strings.TrimSpace(request.URL.Query().Get("dry_run")); queryDryRun != "" {
		importOptions.DryRun = strings.EqualFold(queryDryRun, "true")
	}
	result, err := server.controlService.ImportRules(request.Context(), internalIdentityFromClaims(claims, request), service.RuleImportInput{
		DryRun:     importOptions.DryRun,
		Entry:      raw.Entry,
		Format:     raw.Format,
		SourceText: raw.SourceText,
	})
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleBatchRules(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	var raw validator.RuleBatchRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeJSONDecodeError(response, err)
		return
	}
	normalized, err := validator.ValidateRuleBatchRequest(raw)
	if err != nil {
		writeValidatorError(response, err)
		return
	}
	result, err := server.controlService.BatchRules(request.Context(), internalIdentityFromClaims(claims, request), service.RuleBatchInput{Action: normalized.Action, RuleIDs: normalized.RuleIDs})
	writeServiceResponse(response, http.StatusOK, result, err)
}

func decodeRuleInput(response http.ResponseWriter, request *http.Request) (service.RuleMutationInput, bool) {
	var raw validator.RuleRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeJSONDecodeError(response, err)
		return service.RuleMutationInput{}, false
	}
	normalized, err := validator.ValidateRuleRequest(raw)
	if err != nil {
		writeValidatorError(response, err)
		return service.RuleMutationInput{}, false
	}
	return service.RuleMutationInput{
		Name:                normalized.Name,
		Tags:                normalized.Tags,
		NodeGroupID:         normalized.NodeGroupID,
		ListenIP:            normalized.ListenIP,
		SendIP:              normalized.SendIP,
		FailurePolicy:       normalized.FailurePolicy,
		DataplanePreference: normalized.DataplanePreference,
		ForwardingType:      normalized.ForwardingType,
		Protocol:            normalized.Protocol,
		Port:                normalized.Port,
		PortSegments:        toServiceRulePortSegments(normalized.PortSegments),
		Match: service.RuleMatchInput{
			Type:        normalized.Match.Type,
			SNIHostname: normalized.Match.SNIHostname,
		},
		ProxyProtocol: service.RuleProxyProtocolInput{
			In:  normalized.ProxyProtocol.In,
			Out: normalized.ProxyProtocol.Out,
		},
		Upstream: service.RuleUpstreamInput{
			Type:          normalized.Upstream.Type,
			TargetID:      normalized.Upstream.TargetID,
			TargetGroupID: normalized.Upstream.TargetGroupID,
		},
		Enabled:    normalized.Enabled,
		EnabledSet: true,
	}, true
}

func toServiceRulePortSegments(values []validator.RulePortSegmentRequest) []service.RulePortSegmentInput {
	result := make([]service.RulePortSegmentInput, 0, len(values))
	for _, value := range values {
		result = append(result, service.RulePortSegmentInput{StartPort: value.StartPort, EndPort: value.EndPort})
	}
	return result
}

func decodeRuleCopyInput(response http.ResponseWriter, request *http.Request) (service.RuleCopyInput, bool) {
	var raw validator.RuleCopyRequest
	if request.Body != nil {
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			writeJSONDecodeError(response, err)
			return service.RuleCopyInput{}, false
		}
	}
	normalized, err := validator.ValidateRuleCopyRequest(raw)
	if err != nil {
		writeValidatorError(response, err)
		return service.RuleCopyInput{}, false
	}
	input := service.RuleCopyInput{Name: normalized.Name}
	if normalized.Tags != nil {
		input.Tags = *normalized.Tags
		input.TagsSet = true
	}
	return input, true
}
