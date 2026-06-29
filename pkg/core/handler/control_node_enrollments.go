package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/core/validator"
)

const maxEnrollmentProfileTTLHours = 24 * 366

func decodeNodeEnrollmentProfileInput(response http.ResponseWriter, request *http.Request) (service.NodeEnrollmentProfileMutationInput, bool) {
	var raw struct {
		Name                    string                            `json:"name"`
		Description             string                            `json:"description"`
		Enabled                 *bool                             `json:"enabled"`
		TTLHours                *int                              `json:"ttl_hours"`
		ExpiresAt               string                            `json:"expires_at"`
		MaxUses                 int                               `json:"max_uses"`
		NodeNameTemplate        string                            `json:"node_name_template"`
		GroupIDs                []string                          `json:"group_ids"`
		ListenIPs               []validator.NodeListenIP          `json:"listen_ips"`
		SendIPs                 []validator.NodeSendIP            `json:"send_ips"`
		PortRanges              []validator.NodePortRange         `json:"port_ranges"`
		MaxRulePorts            int                               `json:"max_rule_ports"`
		DNSPublishAddresses     []validator.NodeDNSPublishAddress `json:"dns_publish_addresses"`
		DataplaneMode           string                            `json:"dataplane_mode"`
		DataplaneConflictPolicy string                            `json:"dataplane_conflict_policy"`
		AutoUpdateEnabled       *bool                             `json:"auto_update_enabled"`
		AllowedCIDRs            []string                          `json:"allowed_cidrs"`
	}
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeJSONDecodeError(response, err)
		return service.NodeEnrollmentProfileMutationInput{}, false
	}
	enabled := true
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	}
	autoUpdate := true
	if raw.AutoUpdateEnabled != nil {
		autoUpdate = *raw.AutoUpdateEnabled
	}
	expiresAt := strings.TrimSpace(raw.ExpiresAt)
	ttlHours := 0
	if raw.TTLHours != nil {
		ttlHours = *raw.TTLHours
	}
	expiresAt, ok := decodeNodeEnrollmentProfileExpiresAt(response, expiresAt, ttlHours, raw.TTLHours != nil)
	if !ok {
		return service.NodeEnrollmentProfileMutationInput{}, false
	}
	nodeDefaults, ok := decodeNodeEnrollmentProfileDefaults(response, raw.GroupIDs, raw.ListenIPs, raw.SendIPs, raw.PortRanges, raw.MaxRulePorts, raw.DNSPublishAddresses, raw.DataplaneMode, raw.DataplaneConflictPolicy)
	if !ok {
		return service.NodeEnrollmentProfileMutationInput{}, false
	}
	return service.NodeEnrollmentProfileMutationInput{
		Name:                    raw.Name,
		Description:             raw.Description,
		Enabled:                 enabled,
		ExpiresAt:               expiresAt,
		MaxUses:                 raw.MaxUses,
		NodeNameTemplate:        raw.NodeNameTemplate,
		GroupIDs:                nodeDefaults.GroupIDs,
		ListenIPs:               toServiceListenIPs(nodeDefaults.ListenIPs),
		SendIPs:                 toServiceSendIPs(nodeDefaults.SendIPs),
		PortRanges:              toServicePortRanges(nodeDefaults.PortRanges),
		MaxRulePorts:            nodeDefaults.MaxRulePorts,
		DNSPublishAddresses:     toServiceDNSPublishAddresses(nodeDefaults.DNSPublishAddresses),
		DataplaneMode:           nodeDefaults.DataplaneMode,
		DataplaneConflictPolicy: nodeDefaults.DataplaneConflictPolicy,
		AutoUpdateEnabled:       autoUpdate,
		AllowedCIDRs:            raw.AllowedCIDRs,
	}, true
}

func decodeNodeEnrollmentProfilePatchInput(response http.ResponseWriter, request *http.Request) (service.NodeEnrollmentProfileMutationInput, bool) {
	var raw struct {
		Name                    *string                            `json:"name"`
		Description             *string                            `json:"description"`
		Enabled                 *bool                              `json:"enabled"`
		TTLHours                *int                               `json:"ttl_hours"`
		ExpiresAt               *string                            `json:"expires_at"`
		MaxUses                 *int                               `json:"max_uses"`
		NodeNameTemplate        *string                            `json:"node_name_template"`
		GroupIDs                *[]string                          `json:"group_ids"`
		ListenIPs               *[]validator.NodeListenIP          `json:"listen_ips"`
		SendIPs                 *[]validator.NodeSendIP            `json:"send_ips"`
		PortRanges              *[]validator.NodePortRange         `json:"port_ranges"`
		MaxRulePorts            *int                               `json:"max_rule_ports"`
		DNSPublishAddresses     *[]validator.NodeDNSPublishAddress `json:"dns_publish_addresses"`
		DataplaneMode           *string                            `json:"dataplane_mode"`
		DataplaneConflictPolicy *string                            `json:"dataplane_conflict_policy"`
		AutoUpdateEnabled       *bool                              `json:"auto_update_enabled"`
		AllowedCIDRs            *[]string                          `json:"allowed_cidrs"`
	}
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		writeJSONDecodeError(response, err)
		return service.NodeEnrollmentProfileMutationInput{}, false
	}

	input := service.NodeEnrollmentProfileMutationInput{}
	if raw.Name != nil {
		input.Name = *raw.Name
		input.NameProvided = true
	}
	if raw.Description != nil {
		input.Description = *raw.Description
		input.DescriptionProvided = true
	}
	if raw.Enabled != nil {
		input.Enabled = *raw.Enabled
		input.EnabledProvided = true
	}
	if raw.ExpiresAt != nil || raw.TTLHours != nil {
		ttlHours := 0
		if raw.TTLHours != nil {
			ttlHours = *raw.TTLHours
		}
		expiresAt := ""
		if raw.ExpiresAt != nil {
			expiresAt = strings.TrimSpace(*raw.ExpiresAt)
		}
		if expiresAt == "" && raw.TTLHours != nil && (ttlHours < 1 || ttlHours > maxEnrollmentProfileTTLHours) {
			writeValidationError(response, http.StatusBadRequest, "Node enrollment profile TTL must be between 1 hour and 366 days.", map[string]any{
				"field":  "ttl_hours",
				"actual": ttlHours,
				"min":    1,
				"max":    maxEnrollmentProfileTTLHours,
			})
			return service.NodeEnrollmentProfileMutationInput{}, false
		}
		var ok bool
		input.ExpiresAt, ok = decodeNodeEnrollmentProfileExpiresAt(response, expiresAt, ttlHours, raw.TTLHours != nil)
		if !ok {
			return service.NodeEnrollmentProfileMutationInput{}, false
		}
		input.ExpiresAtProvided = true
	}
	if raw.MaxUses != nil {
		input.MaxUses = *raw.MaxUses
		input.MaxUsesProvided = true
	}
	if raw.NodeNameTemplate != nil {
		input.NodeNameTemplate = *raw.NodeNameTemplate
		input.NodeNameTemplateProvided = true
	}
	if raw.GroupIDs != nil || raw.ListenIPs != nil || raw.SendIPs != nil || raw.PortRanges != nil || raw.MaxRulePorts != nil || raw.DNSPublishAddresses != nil || raw.DataplaneMode != nil || raw.DataplaneConflictPolicy != nil {
		nodePatch := validator.NodePatchRequest{
			GroupIDs:                raw.GroupIDs,
			ListenIPs:               raw.ListenIPs,
			SendIPs:                 raw.SendIPs,
			PortRanges:              raw.PortRanges,
			MaxRulePorts:            raw.MaxRulePorts,
			DNSPublishAddresses:     raw.DNSPublishAddresses,
			DataplaneMode:           raw.DataplaneMode,
			DataplaneConflictPolicy: raw.DataplaneConflictPolicy,
		}
		normalized, err := validator.ValidateNodePatchRequest(nodePatch)
		if err != nil {
			writeValidatorError(response, err)
			return service.NodeEnrollmentProfileMutationInput{}, false
		}
		if normalized.GroupIDs != nil {
			input.GroupIDs = *normalized.GroupIDs
			input.GroupIDsProvided = true
		}
		if normalized.ListenIPs != nil {
			input.ListenIPs = toServiceListenIPs(*normalized.ListenIPs)
			input.ListenIPsProvided = true
		}
		if normalized.SendIPs != nil {
			input.SendIPs = toServiceSendIPs(*normalized.SendIPs)
			input.SendIPsProvided = true
		}
		if normalized.PortRanges != nil {
			input.PortRanges = toServicePortRanges(*normalized.PortRanges)
			input.PortRangesProvided = true
		}
		if normalized.MaxRulePorts != nil {
			input.MaxRulePorts = *normalized.MaxRulePorts
			input.MaxRulePortsProvided = true
		}
		if normalized.DNSPublishAddresses != nil {
			input.DNSPublishAddresses = toServiceDNSPublishAddresses(*normalized.DNSPublishAddresses)
			input.DNSPublishAddressesProvided = true
		}
		if normalized.DataplaneMode != nil {
			input.DataplaneMode = *normalized.DataplaneMode
			input.DataplaneModeProvided = true
		}
		if normalized.DataplaneConflictPolicy != nil {
			input.DataplaneConflictPolicy = *normalized.DataplaneConflictPolicy
			input.DataplaneConflictPolicyProvided = true
		}
	}
	if raw.AutoUpdateEnabled != nil {
		input.AutoUpdateEnabled = *raw.AutoUpdateEnabled
		input.AutoUpdateEnabledProvided = true
	}
	if raw.AllowedCIDRs != nil {
		input.AllowedCIDRs = *raw.AllowedCIDRs
		input.AllowedCIDRsProvided = true
	}
	return input, true
}

func decodeNodeEnrollmentProfileExpiresAt(response http.ResponseWriter, expiresAt string, ttlHours int, ttlHoursProvided bool) (string, bool) {
	if expiresAt == "" && !ttlHoursProvided {
		return "", true
	}
	if expiresAt == "" && (ttlHours < 1 || ttlHours > maxEnrollmentProfileTTLHours) {
		writeValidationError(response, http.StatusBadRequest, "Node enrollment profile TTL must be between 1 hour and 366 days.", map[string]any{
			"field":  "ttl_hours",
			"actual": ttlHours,
			"min":    1,
			"max":    maxEnrollmentProfileTTLHours,
		})
		return "", false
	}
	if expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
		now := time.Now().UTC()
		if err != nil || !parsed.After(now) || parsed.After(now.Add(time.Duration(maxEnrollmentProfileTTLHours)*time.Hour)) {
			writeValidationError(response, http.StatusBadRequest, "Node enrollment profile expiration must be a future time within 366 days.", map[string]any{
				"field": "expires_at",
				"min":   "now",
				"max":   maxEnrollmentProfileTTLHours,
			})
			return "", false
		}
		return expiresAt, true
	}
	return time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339Nano), true
}

func decodeNodeEnrollmentProfileDefaults(
	response http.ResponseWriter,
	groupIDs []string,
	listenIPs []validator.NodeListenIP,
	sendIPs []validator.NodeSendIP,
	portRanges []validator.NodePortRange,
	maxRulePorts int,
	dnsPublishAddresses []validator.NodeDNSPublishAddress,
	dataplaneMode string,
	dataplaneConflictPolicy string,
) (validator.NodeRequest, bool) {
	if len(listenIPs) == 0 {
		listenIPs = []validator.NodeListenIP{{ListenIP: "0.0.0.0", DisplayName: "default"}}
	}
	if len(portRanges) == 0 {
		portRanges = []validator.NodePortRange{{Protocol: "TCP", StartPort: 10000, EndPort: 20000}}
	}
	nodeDefaults, err := validator.ValidateNodeRequest(validator.NodeRequest{
		Name:                    "enrollment-default-node",
		GroupIDs:                groupIDs,
		ListenIPs:               listenIPs,
		SendIPs:                 sendIPs,
		PortRanges:              portRanges,
		MaxRulePorts:            maxRulePorts,
		DNSPublishAddresses:     dnsPublishAddresses,
		DataplaneMode:           dataplaneMode,
		DataplaneConflictPolicy: dataplaneConflictPolicy,
	})
	if err != nil {
		writeValidatorError(response, err)
		return validator.NodeRequest{}, false
	}
	return nodeDefaults, true
}
