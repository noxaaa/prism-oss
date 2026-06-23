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

func (server *ControlServer) handleRefreshDNSCredentialZones(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.RefreshDNSCredentialZones(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("credential_id"))
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

func (server *ControlServer) handleListDNSManagedRecords(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListDNSManagedRecords(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateDNSManagedRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSManagedRecordInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateDNSManagedRecord(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateDNSManagedRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSManagedRecordInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateDNSManagedRecord(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("record_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteDNSManagedRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteDNSManagedRecord(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("record_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func (server *ControlServer) handleEvaluateDNSManagedRecord(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.EvaluateDNSManagedRecord(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("record_id"))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func decodeDNSManagedRecordInput(response http.ResponseWriter, request *http.Request) (service.DNSManagedRecordMutationInput, bool) {
	var raw validator.DNSManagedRecordRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSManagedRecordMutationInput{}, false
	}
	normalized, err := validator.ValidateDNSManagedRecordRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSManagedRecordMutationInput{}, false
	}
	return service.DNSManagedRecordMutationInput{
		DNSCredentialID:  normalized.DNSCredentialID,
		CredentialZoneID: normalized.CredentialZoneID,
		RecordHost:       normalized.RecordHost,
		RecordName:       normalized.RecordName,
		RecordType:       normalized.RecordType,
		TTL:              normalized.TTL,
		Proxied:          normalized.Proxied,
	}, true
}

func (server *ControlServer) handleListDNSInstances(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListDNSInstances(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateDNSInstance(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSInstanceInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.CreateDNSInstance(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateDNSInstance(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeDNSInstanceInput(response, request)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateDNSInstance(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("instance_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteDNSInstance(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteDNSInstance(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("instance_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func decodeDNSInstanceInput(response http.ResponseWriter, request *http.Request) (service.DNSInstanceMutationInput, bool) {
	var raw validator.DNSInstanceRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSInstanceMutationInput{}, false
	}
	normalized, err := validator.ValidateDNSInstanceRequest(raw)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.DNSInstanceMutationInput{}, false
	}
	return service.DNSInstanceMutationInput{
		ManagedRecordID:        normalized.ManagedRecordID,
		Name:                   normalized.Name,
		Priority:               normalized.Priority,
		Enabled:                normalized.Enabled,
		NodeGroupIDs:           normalized.NodeGroupIDs,
		AnswerCount:            normalized.AnswerCount,
		Condition:              normalized.Condition,
		Action:                 normalized.Action,
		NotificationChannelIDs: normalized.NotificationChannelIDs,
	}, true
}

func (server *ControlServer) handleListNotificationChannels(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	result, err := server.controlService.ListNotificationChannels(request.Context(), internalIdentityFromClaims(claims, request))
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleCreateNotificationChannel(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeNotificationChannelInput(response, request, true)
	if !ok {
		return
	}
	result, err := server.controlService.CreateNotificationChannel(request.Context(), internalIdentityFromClaims(claims, request), input)
	writeServiceResponse(response, http.StatusCreated, result, err)
}

func (server *ControlServer) handleUpdateNotificationChannel(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	input, ok := decodeNotificationChannelInput(response, request, false)
	if !ok {
		return
	}
	result, err := server.controlService.UpdateNotificationChannel(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("channel_id"), input)
	writeServiceResponse(response, http.StatusOK, result, err)
}

func (server *ControlServer) handleDeleteNotificationChannel(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
	err := server.controlService.DeleteNotificationChannel(request.Context(), internalIdentityFromClaims(claims, request), request.PathValue("channel_id"))
	writeServiceResponse(response, http.StatusOK, map[string]any{"deleted": true}, err)
}

func decodeNotificationChannelInput(response http.ResponseWriter, request *http.Request, secretRequired bool) (service.NotificationChannelMutationInput, bool) {
	var raw validator.NotificationChannelRequest
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.NotificationChannelMutationInput{}, false
	}
	normalized, err := validator.ValidateNotificationChannelRequest(raw, secretRequired)
	if err != nil {
		writeError(response, http.StatusBadRequest, "VALIDATION_FAILED")
		return service.NotificationChannelMutationInput{}, false
	}
	return service.NotificationChannelMutationInput{
		Name:        normalized.Name,
		ChannelType: normalized.ChannelType,
		Config:      normalized.Config,
		Secret:      normalized.Secret,
		Enabled:     normalized.Enabled,
	}, true
}
