package handler

import (
	"encoding/json"
	"net/http"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/core/validator"
)

func (server *ControlServer) handleListHealthChecks(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListHealthChecks(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateHealthCheck(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeHealthCheckInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateHealthCheck(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateHealthCheck(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeHealthCheckInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateHealthCheck(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("health_check_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteHealthCheck(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteHealthCheck(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("health_check_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleListHealthCheckResults(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListHealthResults(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("health_check_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func decodeHealthCheckInput(response http.ResponseWriter, request *http.Request) (service.HealthCheckMutationInput, bool) {
	var raw validator.HealthCheckRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.HealthCheckMutationInput{}, false
	}
	normalized, err := validator.ValidateHealthCheckRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.HealthCheckMutationInput{}, false
	}
	configJSON, err := json.Marshal(normalized.Config)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.HealthCheckMutationInput{}, false
	}
	return service.HealthCheckMutationInput{
		Name:            normalized.Name,
		ProbeType:       normalized.ProbeType,
		IntervalSeconds: normalized.IntervalSeconds,
		TimeoutSeconds:  normalized.TimeoutSeconds,
		Enabled:         normalized.Enabled,
		TargetScope: service.HealthTargetScopeInput{
			Type:          normalized.TargetScope.Type,
			TargetIDs:     normalized.TargetScope.TargetIDs,
			TargetGroupID: normalized.TargetScope.TargetGroupID,
		},
		MonitorScope: service.HealthMonitorScopeInput{
			Type:           normalized.MonitorScope.Type,
			MonitorID:      normalized.MonitorScope.MonitorID,
			MonitorGroupID: normalized.MonitorScope.MonitorGroupID,
		},
		ConfigJSON: string(configJSON),
	}, true
}

func (server *ControlServer) handleListDNSCredentials(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListDNSCredentials(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateDNSCredential(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSCredentialInput(response, request, true)
	if !ok {
		return
	}
	result, err := server.controlService.CreateDNSCredential(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateDNSCredential(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSCredentialInput(response, request, false)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateDNSCredential(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("credential_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteDNSCredential(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteDNSCredential(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("credential_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func decodeDNSCredentialInput(response http.ResponseWriter, request *http.Request, secretRequired bool) (service.DNSCredentialMutationInput, bool) {
	var raw validator.DNSCredentialRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSCredentialMutationInput{}, false
	}
	normalized, err := validator.ValidateDNSCredentialRequest(raw, secretRequired)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSCredentialMutationInput{}, false
	}
	return service.DNSCredentialMutationInput{Name: normalized.Name, Provider: normalized.Provider, Secret: normalized.Secret}, true
}

func (server *ControlServer) handleListDNSRecords(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListDNSRecords(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateDNSRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSRecordInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateDNSRecord(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateDNSRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSRecordInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateDNSRecord(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("record_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteDNSRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteDNSRecord(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("record_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func decodeDNSRecordInput(response http.ResponseWriter, request *http.Request) (service.DNSRecordMutationInput, bool) {
	var raw validator.DNSRecordRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSRecordMutationInput{}, false
	}
	normalized, err := validator.ValidateDNSRecordRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSRecordMutationInput{}, false
	}
	return service.DNSRecordMutationInput{
		DNSCredentialID: normalized.DNSCredentialID,
		Zone:            normalized.Zone,
		RecordName:      normalized.RecordName,
		RecordType:      normalized.RecordType,
		DesiredValues:   normalized.DesiredValues,
		HealthCheckID:   normalized.HealthCheckID,
		EventType:       normalized.EventType,
		FailoverValues:  normalized.FailoverValues,
	}, true
}
