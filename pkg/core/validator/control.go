package validator

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"sort"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid request")

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
var hostnamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*\.?$`)

type ValidationError struct {
	Message string
	Details map[string]any
}

func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *ValidationError) Unwrap() error {
	return ErrInvalidRequest
}

func invalidFieldError(field string, message string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["field"] = field
	return invalidRequestError(message, details)
}

func invalidRequestError(message string, details map[string]any) error {
	if message == "" {
		message = "The request payload is invalid."
	}
	copied := map[string]any(nil)
	if len(details) > 0 {
		copied = make(map[string]any, len(details))
		for key, value := range details {
			copied[key] = value
		}
	}
	return &ValidationError{Message: message, Details: copied}
}

type BootstrapRequest struct {
	OrganizationName string `json:"organization_name"`
	OrganizationSlug string `json:"organization_slug"`
}

type OrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type MemberRequest struct {
	Email   string   `json:"email"`
	RoleIDs []string `json:"role_ids"`
	Status  string   `json:"status"`
}

type MemberUpdateRequest struct {
	RoleIDs *[]string `json:"role_ids"`
	Status  string    `json:"status"`
}

type RoleRequest struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Permissions    []string               `json:"permissions"`
	ResourceScopes []ResourceScopeRequest `json:"resource_scopes"`
}

type GroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type NodeRequest struct {
	Name                    string                  `json:"name"`
	GroupIDs                []string                `json:"group_ids"`
	ListenIPs               []NodeListenIP          `json:"listen_ips"`
	SendIPs                 []NodeSendIP            `json:"send_ips"`
	PortRanges              []NodePortRange         `json:"port_ranges"`
	MaxRulePorts            int                     `json:"max_rule_ports"`
	DNSPublishAddresses     []NodeDNSPublishAddress `json:"dns_publish_addresses"`
	PublicDescription       string                  `json:"public_description"`
	DataplaneMode           string                  `json:"dataplane_mode"`
	DataplaneConflictPolicy string                  `json:"dataplane_conflict_policy"`
}

type NodePatchRequest struct {
	Name                    *string                  `json:"name"`
	GroupIDs                *[]string                `json:"group_ids"`
	ListenIPs               *[]NodeListenIP          `json:"listen_ips"`
	SendIPs                 *[]NodeSendIP            `json:"send_ips"`
	PortRanges              *[]NodePortRange         `json:"port_ranges"`
	MaxRulePorts            *int                     `json:"max_rule_ports"`
	DNSPublishAddresses     *[]NodeDNSPublishAddress `json:"dns_publish_addresses"`
	PublicDescription       *string                  `json:"public_description"`
	DataplaneMode           *string                  `json:"dataplane_mode"`
	DataplaneConflictPolicy *string                  `json:"dataplane_conflict_policy"`
}

type NodeListenIP struct {
	ListenIP    string `json:"listen_ip"`
	DisplayName string `json:"display_name"`
}

type NodeSendIP struct {
	SendIP      string `json:"send_ip"`
	DisplayName string `json:"display_name"`
}

type NodePortRange struct {
	Protocol  string `json:"protocol"`
	StartPort int    `json:"start_port"`
	EndPort   int    `json:"end_port"`
}

type NodeDNSPublishAddress struct {
	AddressType string `json:"address_type"`
	Address     string `json:"address"`
	Enabled     bool   `json:"enabled"`
}

func (address *NodeDNSPublishAddress) UnmarshalJSON(data []byte) error {
	var raw struct {
		AddressType string `json:"address_type"`
		Address     string `json:"address"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	address.AddressType = raw.AddressType
	address.Address = raw.Address
	address.Enabled = true
	if raw.Enabled != nil {
		address.Enabled = *raw.Enabled
	}
	return nil
}

type MonitorRequest struct {
	Name     string   `json:"name"`
	GroupIDs []string `json:"group_ids"`
}

type MonitorPatchRequest struct {
	Name     *string   `json:"name"`
	GroupIDs *[]string `json:"group_ids"`
}

type HealthCheckRequest struct {
	Name            string                    `json:"name"`
	ProbeType       string                    `json:"probe_type"`
	IntervalSeconds int                       `json:"interval_seconds"`
	TimeoutSeconds  int                       `json:"timeout_seconds"`
	Enabled         bool                      `json:"enabled"`
	TargetScope     HealthTargetScopeRequest  `json:"target_scope"`
	MonitorScope    HealthMonitorScopeRequest `json:"monitor_scope"`
	Config          map[string]any            `json:"config"`
}

type HealthTargetScopeRequest struct {
	Type          string   `json:"type"`
	TargetIDs     []string `json:"target_ids"`
	TargetGroupID string   `json:"target_group_id"`
}

type HealthMonitorScopeRequest struct {
	Type           string `json:"type"`
	MonitorID      string `json:"monitor_id"`
	MonitorGroupID string `json:"monitor_group_id"`
}

type DNSCredentialRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Secret   string `json:"secret"`
}

type DNSManagedRecordRequest struct {
	DNSCredentialID  string `json:"dns_credential_id"`
	CredentialZoneID string `json:"credential_zone_id"`
	RecordHost       string `json:"record_host"`
	RecordName       string `json:"record_name"`
	RecordType       string `json:"record_type"`
	TTL              int    `json:"ttl"`
	Proxied          bool   `json:"proxied"`
}

type DNSInstanceRequest struct {
	ManagedRecordID        string         `json:"managed_record_id"`
	Name                   string         `json:"name"`
	Priority               int            `json:"priority"`
	Enabled                bool           `json:"enabled"`
	NodeGroupIDs           []string       `json:"node_group_ids"`
	AnswerCount            int            `json:"answer_count"`
	Condition              map[string]any `json:"condition"`
	Action                 map[string]any `json:"action"`
	NotificationChannelIDs []string       `json:"notification_channel_ids"`
}

type NotificationChannelRequest struct {
	Name        string         `json:"name"`
	ChannelType string         `json:"channel_type"`
	Config      map[string]any `json:"config"`
	Secret      string         `json:"secret"`
	Enabled     bool           `json:"enabled"`
}

type RegistrationTokenRequest struct {
	TTLHours int `json:"ttl_hours"`
}

type TargetRequest struct {
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	Enabled        bool      `json:"enabled"`
	TargetGroupIDs *[]string `json:"target_group_ids"`
}

type TargetGroupRequest struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Scheduler   string                     `json:"scheduler"`
	Members     []TargetGroupMemberRequest `json:"members"`
}

type TargetGroupMemberRequest struct {
	TargetID string `json:"target_id"`
	Priority int    `json:"priority"`
	Weight   *int   `json:"weight,omitempty"`
	Enabled  bool   `json:"enabled"`
}

type RuleRequest struct {
	Name                string                   `json:"name"`
	Tags                []string                 `json:"tags"`
	NodeGroupID         string                   `json:"node_group_id"`
	ListenIP            string                   `json:"listen_ip"`
	SendIP              string                   `json:"send_ip"`
	FailurePolicy       string                   `json:"failure_policy"`
	DataplanePreference string                   `json:"dataplane_preference"`
	ForwardingType      string                   `json:"forwarding_type"`
	Protocol            string                   `json:"protocol"`
	Port                int                      `json:"port"`
	PortSegments        []RulePortSegmentRequest `json:"port_segments"`
	Match               RuleMatchRequest         `json:"match"`
	ProxyProtocol       ProxyProtocolRequest     `json:"proxy_protocol"`
	Upstream            RuleUpstreamRequest      `json:"upstream"`
	Enabled             bool                     `json:"enabled"`
}

type RulePortSegmentRequest struct {
	StartPort int `json:"start_port"`
	EndPort   int `json:"end_port"`
}

type RuleMatchRequest struct {
	Type        string `json:"type"`
	SNIHostname string `json:"sni_hostname"`
}

type ProxyProtocolRequest struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

type RuleUpstreamRequest struct {
	Type          string `json:"type"`
	TargetID      string `json:"target_id"`
	TargetGroupID string `json:"target_group_id"`
}

type RuleCopyRequest struct {
	Name string    `json:"name"`
	Tags *[]string `json:"tags"`
}

type RuleImportRequest struct {
	DryRun bool `json:"dry_run"`
}

type RuleBatchRequest struct {
	Action  string   `json:"action"`
	RuleIDs []string `json:"rule_ids"`
}

type ResourceScopeRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	AccessLevel  string `json:"access_level"`
}

func ValidateBootstrapRequest(request BootstrapRequest) (BootstrapRequest, error) {
	request.OrganizationName = strings.TrimSpace(request.OrganizationName)
	request.OrganizationSlug = normalizeSlug(request.OrganizationSlug)
	if request.OrganizationName == "" || len(request.OrganizationName) > 120 {
		return BootstrapRequest{}, ErrInvalidRequest
	}
	if !slugPattern.MatchString(request.OrganizationSlug) {
		return BootstrapRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateOrganizationRequest(request OrganizationRequest) (OrganizationRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Slug = normalizeSlug(request.Slug)
	if request.Name == "" || len(request.Name) > 120 || !slugPattern.MatchString(request.Slug) {
		return OrganizationRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateMemberRequest(request MemberRequest) (MemberRequest, error) {
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.Status = strings.ToUpper(strings.TrimSpace(request.Status))
	request.RoleIDs = normalizeIDs(request.RoleIDs)
	if request.Email == "" || !strings.Contains(request.Email, "@") {
		return MemberRequest{}, ErrInvalidRequest
	}
	if request.Status != "" && request.Status != "ACTIVE" && request.Status != "DISABLED" {
		return MemberRequest{}, ErrInvalidRequest
	}
	if len(request.RoleIDs) == 0 {
		return MemberRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateMemberUpdateRequest(request MemberUpdateRequest) (MemberUpdateRequest, error) {
	request.Status = strings.ToUpper(strings.TrimSpace(request.Status))
	if request.RoleIDs != nil {
		normalized := normalizeIDs(*request.RoleIDs)
		request.RoleIDs = &normalized
	}
	if request.Status != "" && request.Status != "ACTIVE" && request.Status != "DISABLED" {
		return MemberUpdateRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateRoleRequest(request RoleRequest) (RoleRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Permissions = normalizeIDs(request.Permissions)
	if request.Name == "" || len(request.Name) > 120 || len(request.Permissions) == 0 {
		return RoleRequest{}, ErrInvalidRequest
	}
	for index, scope := range request.ResourceScopes {
		scope.ResourceType = strings.ToUpper(strings.TrimSpace(scope.ResourceType))
		scope.ResourceID = strings.TrimSpace(scope.ResourceID)
		scope.AccessLevel = strings.ToUpper(strings.TrimSpace(scope.AccessLevel))
		if scope.ResourceType == "" || scope.ResourceID == "" || scope.AccessLevel == "" {
			return RoleRequest{}, ErrInvalidRequest
		}
		request.ResourceScopes[index] = scope
	}
	return request, nil
}

func ValidateGroupRequest(request GroupRequest) (GroupRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if request.Name == "" || len(request.Name) > 120 || len(request.Description) > 1000 {
		return GroupRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateNodeRequest(request NodeRequest) (NodeRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.PublicDescription = strings.TrimSpace(request.PublicDescription)
	request.DataplaneMode = normalizeDataplaneMode(request.DataplaneMode)
	request.DataplaneConflictPolicy = normalizeDataplaneConflictPolicy(request.DataplaneConflictPolicy)
	request.GroupIDs = normalizeIDs(request.GroupIDs)
	if request.Name == "" {
		return NodeRequest{}, invalidFieldError("name", "Node name is required.", nil)
	}
	if len(request.Name) > 120 {
		return NodeRequest{}, invalidFieldError("name", "Node name must be at most 120 characters.", nil)
	}
	if len(request.PublicDescription) > 2000 {
		return NodeRequest{}, invalidFieldError("public_description", "Public description must be at most 2000 characters.", nil)
	}
	if !validDataplaneMode(request.DataplaneMode) {
		return NodeRequest{}, invalidFieldError("dataplane_mode", "Dataplane mode is not supported.", map[string]any{"actual": request.DataplaneMode})
	}
	if !validDataplaneConflictPolicy(request.DataplaneConflictPolicy) {
		return NodeRequest{}, invalidFieldError("dataplane_conflict_policy", "Dataplane conflict policy is not supported.", map[string]any{"actual": request.DataplaneConflictPolicy})
	}
	listenIPs, err := validateListenIPs(request.ListenIPs)
	if err != nil {
		return NodeRequest{}, err
	}
	sendIPs, err := validateSendIPs(request.SendIPs)
	if err != nil {
		return NodeRequest{}, err
	}
	portRanges, err := validatePortRanges(request.PortRanges)
	if err != nil {
		return NodeRequest{}, err
	}
	maxRulePorts, err := validateMaxRulePorts(request.MaxRulePorts)
	if err != nil {
		return NodeRequest{}, err
	}
	dnsPublishAddresses, err := validateDNSPublishAddresses(request.DNSPublishAddresses)
	if err != nil {
		return NodeRequest{}, err
	}
	request.ListenIPs = listenIPs
	request.SendIPs = sendIPs
	request.PortRanges = portRanges
	request.MaxRulePorts = maxRulePorts
	request.DNSPublishAddresses = dnsPublishAddresses
	return request, nil
}

func ValidateNodePatchRequest(request NodePatchRequest) (NodePatchRequest, error) {
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return NodePatchRequest{}, invalidFieldError("name", "Node name is required.", nil)
		}
		if len(name) > 120 {
			return NodePatchRequest{}, invalidFieldError("name", "Node name must be at most 120 characters.", nil)
		}
		request.Name = &name
	}
	if request.PublicDescription != nil {
		description := strings.TrimSpace(*request.PublicDescription)
		if len(description) > 2000 {
			return NodePatchRequest{}, invalidFieldError("public_description", "Public description must be at most 2000 characters.", nil)
		}
		request.PublicDescription = &description
	}
	if request.DataplaneMode != nil {
		mode := normalizeDataplaneMode(*request.DataplaneMode)
		if !validDataplaneMode(mode) {
			return NodePatchRequest{}, invalidFieldError("dataplane_mode", "Dataplane mode is not supported.", map[string]any{"actual": mode})
		}
		request.DataplaneMode = &mode
	}
	if request.DataplaneConflictPolicy != nil {
		policy := normalizeDataplaneConflictPolicy(*request.DataplaneConflictPolicy)
		if !validDataplaneConflictPolicy(policy) {
			return NodePatchRequest{}, invalidFieldError("dataplane_conflict_policy", "Dataplane conflict policy is not supported.", map[string]any{"actual": policy})
		}
		request.DataplaneConflictPolicy = &policy
	}
	if request.GroupIDs != nil {
		groupIDs := normalizeIDs(*request.GroupIDs)
		request.GroupIDs = &groupIDs
	}
	if request.ListenIPs != nil {
		listenIPs, err := validateListenIPs(*request.ListenIPs)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.ListenIPs = &listenIPs
	}
	if request.SendIPs != nil {
		sendIPs, err := validateSendIPs(*request.SendIPs)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.SendIPs = &sendIPs
	}
	if request.PortRanges != nil {
		portRanges, err := validatePortRanges(*request.PortRanges)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.PortRanges = &portRanges
	}
	if request.MaxRulePorts != nil {
		maxRulePorts, err := validateMaxRulePorts(*request.MaxRulePorts)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.MaxRulePorts = &maxRulePorts
	}
	if request.DNSPublishAddresses != nil {
		dnsPublishAddresses, err := validateDNSPublishAddresses(*request.DNSPublishAddresses)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.DNSPublishAddresses = &dnsPublishAddresses
	}
	return request, nil
}

func normalizeDataplaneMode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "AUTO"
	}
	return value
}

func normalizeDataplaneConflictPolicy(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "FAIL_FAST"
	}
	return value
}

func validDataplaneMode(value string) bool {
	switch value {
	case "AUTO", "NATIVE", "HAPROXY", "NFTABLES":
		return true
	default:
		return false
	}
}

func validDataplaneConflictPolicy(value string) bool {
	return value == "FAIL_FAST"
}

func ValidateMonitorRequest(request MonitorRequest) (MonitorRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.GroupIDs = normalizeIDs(request.GroupIDs)
	if request.Name == "" || len(request.Name) > 120 || len(request.GroupIDs) == 0 {
		return MonitorRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateMonitorPatchRequest(request MonitorPatchRequest) (MonitorPatchRequest, error) {
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || len(name) > 120 {
			return MonitorPatchRequest{}, ErrInvalidRequest
		}
		request.Name = &name
	}
	if request.GroupIDs != nil {
		groupIDs := normalizeIDs(*request.GroupIDs)
		request.GroupIDs = &groupIDs
	}
	return request, nil
}

func ValidateHealthCheckRequest(request HealthCheckRequest) (HealthCheckRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.ProbeType = strings.ToUpper(strings.TrimSpace(request.ProbeType))
	if request.Name == "" || len(request.Name) > 120 {
		return HealthCheckRequest{}, ErrInvalidRequest
	}
	if request.ProbeType != "ICMP" && request.ProbeType != "TCP_PORT" && request.ProbeType != "HTTP" {
		return HealthCheckRequest{}, ErrInvalidRequest
	}
	if request.IntervalSeconds <= 0 || request.TimeoutSeconds <= 0 || request.TimeoutSeconds > request.IntervalSeconds {
		return HealthCheckRequest{}, ErrInvalidRequest
	}
	targetScope, err := validateHealthTargetScope(request.TargetScope)
	if err != nil {
		return HealthCheckRequest{}, err
	}
	monitorScope, err := validateHealthMonitorScope(request.MonitorScope)
	if err != nil {
		return HealthCheckRequest{}, err
	}
	if request.Config == nil {
		request.Config = map[string]any{}
	}
	request.TargetScope = targetScope
	request.MonitorScope = monitorScope
	return request, nil
}

func validateHealthTargetScope(scope HealthTargetScopeRequest) (HealthTargetScopeRequest, error) {
	scope.Type = strings.ToUpper(strings.TrimSpace(scope.Type))
	scope.TargetIDs = normalizeIDs(scope.TargetIDs)
	scope.TargetGroupID = strings.TrimSpace(scope.TargetGroupID)
	switch scope.Type {
	case "TARGETS":
		if len(scope.TargetIDs) == 0 || scope.TargetGroupID != "" {
			return HealthTargetScopeRequest{}, ErrInvalidRequest
		}
	case "TARGET_GROUP":
		if scope.TargetGroupID == "" || len(scope.TargetIDs) != 0 {
			return HealthTargetScopeRequest{}, ErrInvalidRequest
		}
	default:
		return HealthTargetScopeRequest{}, ErrInvalidRequest
	}
	return scope, nil
}

func validateHealthMonitorScope(scope HealthMonitorScopeRequest) (HealthMonitorScopeRequest, error) {
	scope.Type = strings.ToUpper(strings.TrimSpace(scope.Type))
	scope.MonitorID = strings.TrimSpace(scope.MonitorID)
	scope.MonitorGroupID = strings.TrimSpace(scope.MonitorGroupID)
	switch scope.Type {
	case "MONITOR":
		if scope.MonitorID == "" || scope.MonitorGroupID != "" {
			return HealthMonitorScopeRequest{}, ErrInvalidRequest
		}
	case "MONITOR_GROUP":
		if scope.MonitorGroupID == "" || scope.MonitorID != "" {
			return HealthMonitorScopeRequest{}, ErrInvalidRequest
		}
	default:
		return HealthMonitorScopeRequest{}, ErrInvalidRequest
	}
	return scope, nil
}

func ValidateDNSCredentialRequest(request DNSCredentialRequest, secretRequired bool) (DNSCredentialRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Provider = strings.ToUpper(strings.TrimSpace(request.Provider))
	request.Secret = strings.TrimSpace(request.Secret)
	if request.Name == "" || len(request.Name) > 120 || request.Provider != "CLOUDFLARE" {
		return DNSCredentialRequest{}, ErrInvalidRequest
	}
	if secretRequired && request.Secret == "" {
		return DNSCredentialRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateDNSManagedRecordRequest(request DNSManagedRecordRequest) (DNSManagedRecordRequest, error) {
	request.DNSCredentialID = strings.TrimSpace(request.DNSCredentialID)
	request.CredentialZoneID = strings.TrimSpace(request.CredentialZoneID)
	request.RecordHost = strings.TrimSpace(request.RecordHost)
	request.RecordName = strings.TrimSpace(request.RecordName)
	request.RecordType = strings.ToUpper(strings.TrimSpace(request.RecordType))
	if request.DNSCredentialID == "" || request.CredentialZoneID == "" {
		return DNSManagedRecordRequest{}, ErrInvalidRequest
	}
	if request.RecordType != "A" && request.RecordType != "AAAA" && request.RecordType != "CNAME" {
		return DNSManagedRecordRequest{}, ErrInvalidRequest
	}
	if request.TTL < 0 {
		return DNSManagedRecordRequest{}, ErrInvalidRequest
	}
	if request.TTL == 0 {
		request.TTL = 60
	}
	return request, nil
}

func ValidateDNSInstanceRequest(request DNSInstanceRequest) (DNSInstanceRequest, error) {
	request.ManagedRecordID = strings.TrimSpace(request.ManagedRecordID)
	request.Name = strings.TrimSpace(request.Name)
	if request.ManagedRecordID == "" || request.Name == "" || len(request.Name) > 120 || request.Priority < 0 {
		return DNSInstanceRequest{}, ErrInvalidRequest
	}
	if request.AnswerCount == 0 {
		request.AnswerCount = -1
	}
	if request.AnswerCount < -1 {
		return DNSInstanceRequest{}, ErrInvalidRequest
	}
	request.NodeGroupIDs = uniqueNonEmptyStrings(request.NodeGroupIDs)
	request.NotificationChannelIDs = uniqueNonEmptyStrings(request.NotificationChannelIDs)
	if request.Condition == nil {
		request.Condition = map[string]any{}
	}
	if request.Action == nil {
		return DNSInstanceRequest{}, ErrInvalidRequest
	}
	actionType, _ := request.Action["type"].(string)
	if strings.TrimSpace(actionType) == "" {
		return DNSInstanceRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateNotificationChannelRequest(request NotificationChannelRequest, secretRequired bool) (NotificationChannelRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.ChannelType = strings.ToUpper(strings.TrimSpace(request.ChannelType))
	request.Secret = strings.TrimSpace(request.Secret)
	if request.Name == "" || len(request.Name) > 120 {
		return NotificationChannelRequest{}, ErrInvalidRequest
	}
	if request.ChannelType != "WEBHOOK" && request.ChannelType != "EMAIL" {
		return NotificationChannelRequest{}, ErrInvalidRequest
	}
	if request.Config == nil {
		request.Config = map[string]any{}
	}
	if request.ChannelType == "EMAIL" && secretRequired && request.Secret == "" {
		return NotificationChannelRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func ValidateRegistrationTokenRequest(request RegistrationTokenRequest) (RegistrationTokenRequest, error) {
	if request.TTLHours < 0 || request.TTLHours > 24*7 {
		return RegistrationTokenRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateTargetRequest(request TargetRequest) (TargetRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Host = strings.TrimSpace(request.Host)
	if request.TargetGroupIDs != nil {
		targetGroupIDs := normalizeIDs(*request.TargetGroupIDs)
		request.TargetGroupIDs = &targetGroupIDs
	}
	if request.Name == "" || len(request.Name) > 120 || request.Host == "" || len(request.Host) > 253 {
		return TargetRequest{}, ErrInvalidRequest
	}
	if strings.ContainsAny(request.Host, " \t\r\n") || request.Port < 1 || request.Port > 65535 {
		return TargetRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func ValidateTargetGroupRequest(request TargetGroupRequest) (TargetGroupRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Scheduler = strings.ToUpper(strings.TrimSpace(request.Scheduler))
	if request.Scheduler == "" {
		request.Scheduler = "PRIORITY_IPHASH"
	}
	if request.Name == "" || len(request.Name) > 120 || len(request.Description) > 1000 {
		return TargetGroupRequest{}, ErrInvalidRequest
	}
	seen := map[string]bool{}
	for index, member := range request.Members {
		member.TargetID = strings.TrimSpace(member.TargetID)
		if member.TargetID == "" || member.Priority < 0 || seen[member.TargetID] {
			return TargetGroupRequest{}, ErrInvalidRequest
		}
		weight := 1
		if member.Weight != nil {
			weight = *member.Weight
		}
		if weight < 0 || weight > 256 {
			return TargetGroupRequest{}, ErrInvalidRequest
		}
		member.Weight = &weight
		seen[member.TargetID] = true
		request.Members[index] = member
	}
	return request, nil
}

func ValidateRuleRequest(request RuleRequest) (RuleRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.NodeGroupID = strings.TrimSpace(request.NodeGroupID)
	request.ListenIP = strings.TrimSpace(request.ListenIP)
	request.SendIP = strings.TrimSpace(request.SendIP)
	request.FailurePolicy = strings.ToUpper(strings.TrimSpace(request.FailurePolicy))
	request.DataplanePreference = strings.ToUpper(strings.TrimSpace(request.DataplanePreference))
	request.ForwardingType = strings.ToUpper(strings.TrimSpace(request.ForwardingType))
	request.Protocol = strings.ToUpper(strings.TrimSpace(request.Protocol))
	request.Match.Type = strings.ToUpper(strings.TrimSpace(request.Match.Type))
	request.Match.SNIHostname = strings.ToLower(strings.TrimSpace(request.Match.SNIHostname))
	request.ProxyProtocol.In = strings.ToUpper(strings.TrimSpace(request.ProxyProtocol.In))
	request.ProxyProtocol.Out = strings.ToUpper(strings.TrimSpace(request.ProxyProtocol.Out))
	request.Upstream.Type = strings.ToUpper(strings.TrimSpace(request.Upstream.Type))
	request.Upstream.TargetID = strings.TrimSpace(request.Upstream.TargetID)
	request.Upstream.TargetGroupID = strings.TrimSpace(request.Upstream.TargetGroupID)
	request.Tags = normalizeTags(request.Tags)
	if request.Name == "" {
		return RuleRequest{}, invalidFieldError("name", "Rule name is required.", nil)
	}
	if len(request.Name) > 120 {
		return RuleRequest{}, invalidFieldError("name", "Rule name must be at most 120 characters.", map[string]any{"max_length": 120})
	}
	if request.NodeGroupID == "" {
		return RuleRequest{}, invalidFieldError("node_group_id", "Rule node_group_id is required.", nil)
	}
	if request.ListenIP == "" {
		return RuleRequest{}, invalidFieldError("listen_ip", "Rule listen_ip is required.", nil)
	}
	segments, err := validateRulePortSegments(request.Port, request.PortSegments)
	if err != nil {
		return RuleRequest{}, err
	}
	request.PortSegments = segments
	request.Port = segments[0].StartPort
	if request.SendIP != "" && net.ParseIP(request.SendIP) == nil {
		return RuleRequest{}, invalidFieldError("send_ip", "Rule send_ip must be a valid IP address.", nil)
	}
	if request.Protocol != "TCP" && request.Protocol != "UDP" && request.Protocol != "TCP_UDP" {
		return RuleRequest{}, invalidFieldError("protocol", "Rule protocol must be TCP, UDP, or TCP_UDP.", map[string]any{"actual": request.Protocol})
	}
	if request.ForwardingType == "" {
		request.ForwardingType = "DIRECT"
	}
	if request.ForwardingType != "DIRECT" {
		return RuleRequest{}, invalidFieldError("forwarding_type", "Only DIRECT forwarding is supported by the current runtime.", map[string]any{"actual": request.ForwardingType})
	}
	if request.FailurePolicy == "" {
		request.FailurePolicy = "KEEP_ENABLED"
	}
	if request.FailurePolicy != "KEEP_ENABLED" && request.FailurePolicy != "DISABLE_WHEN_ALL_NODES_FAILED" {
		return RuleRequest{}, invalidFieldError("failure_policy", "Unsupported rule failure policy.", map[string]any{"actual": request.FailurePolicy})
	}
	if request.DataplanePreference == "" {
		request.DataplanePreference = "AUTO"
	}
	if request.DataplanePreference != "AUTO" && request.DataplanePreference != "NATIVE" && request.DataplanePreference != "HAPROXY" && request.DataplanePreference != "NFTABLES" {
		return RuleRequest{}, invalidFieldError("dataplane_preference", "Unsupported rule dataplane preference.", map[string]any{"actual": request.DataplanePreference})
	}
	if request.Match.Type != "ANY_INBOUND" && request.Match.Type != "TLS_SNI" {
		return RuleRequest{}, invalidFieldError("match.type", "Rule match type must be ANY_INBOUND or TLS_SNI.", map[string]any{"actual": request.Match.Type})
	}
	if request.Protocol != "TCP" && request.Match.Type != "ANY_INBOUND" {
		return RuleRequest{}, invalidFieldError("match.type", "Only TCP rules can use TLS_SNI matching.", map[string]any{"match_type": request.Match.Type, "protocol": request.Protocol})
	}
	if request.Match.Type == "TLS_SNI" && request.Match.SNIHostname == "" {
		return RuleRequest{}, invalidFieldError("match.sni_hostname", "TLS_SNI rules require an SNI hostname.", nil)
	}
	if request.Match.Type == "TLS_SNI" && !validSNIHostname(request.Match.SNIHostname) {
		return RuleRequest{}, invalidFieldError("match.sni_hostname", "TLS_SNI hostname must be a valid hostname token.", nil)
	}
	if request.Match.Type == "ANY_INBOUND" {
		request.Match.SNIHostname = ""
	}
	if !validProxyProtocol(request.ProxyProtocol.In) || !validProxyProtocol(request.ProxyProtocol.Out) {
		return RuleRequest{}, invalidRequestError("Rule proxy protocol must be NONE, V1, or V2.", map[string]any{"proxy_protocol.in": request.ProxyProtocol.In, "proxy_protocol.out": request.ProxyProtocol.Out})
	}
	if request.Protocol == "UDP" && (normalizedProxyProtocol(request.ProxyProtocol.In) != "" || normalizedProxyProtocol(request.ProxyProtocol.Out) != "") {
		return RuleRequest{}, invalidRequestError("UDP rules cannot use Proxy Protocol.", map[string]any{"protocol": request.Protocol, "proxy_protocol.in": request.ProxyProtocol.In, "proxy_protocol.out": request.ProxyProtocol.Out})
	}
	switch request.Upstream.Type {
	case "TARGET":
		if request.Upstream.TargetID == "" || request.Upstream.TargetGroupID != "" {
			return RuleRequest{}, invalidRequestError("TARGET upstream requires target_id and no target_group_id.", map[string]any{"upstream.target_id_present": request.Upstream.TargetID != "", "upstream.target_group_id_present": request.Upstream.TargetGroupID != ""})
		}
	case "TARGET_GROUP":
		if request.Upstream.TargetGroupID == "" || request.Upstream.TargetID != "" {
			return RuleRequest{}, invalidRequestError("TARGET_GROUP upstream requires target_group_id and no target_id.", map[string]any{"upstream.target_id_present": request.Upstream.TargetID != "", "upstream.target_group_id_present": request.Upstream.TargetGroupID != ""})
		}
	default:
		return RuleRequest{}, invalidFieldError("upstream.type", "Rule upstream type must be TARGET or TARGET_GROUP.", map[string]any{"actual": request.Upstream.Type})
	}
	return request, nil
}

func validSNIHostname(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || len(value) > 253 {
		return false
	}
	return hostnamePattern.MatchString(value)
}

func ValidateRuleCopyRequest(request RuleCopyRequest) (RuleCopyRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	if request.Tags != nil {
		tags := normalizeTags(*request.Tags)
		request.Tags = &tags
	}
	if len(request.Name) > 120 {
		return RuleCopyRequest{}, invalidFieldError("name", "Copied rule name must be at most 120 characters.", map[string]any{"max_length": 120})
	}
	return request, nil
}

func ValidateRuleBatchRequest(request RuleBatchRequest) (RuleBatchRequest, error) {
	request.Action = strings.ToUpper(strings.TrimSpace(request.Action))
	if request.Action != "ENABLE" && request.Action != "DISABLE" && request.Action != "DELETE" {
		return RuleBatchRequest{}, invalidFieldError("action", "Batch action must be ENABLE, DISABLE, or DELETE.", map[string]any{"actual": request.Action})
	}
	seen := map[string]bool{}
	ruleIDs := make([]string, 0, len(request.RuleIDs))
	for _, ruleID := range request.RuleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID == "" || seen[ruleID] {
			continue
		}
		seen[ruleID] = true
		ruleIDs = append(ruleIDs, ruleID)
	}
	if len(ruleIDs) == 0 {
		return RuleBatchRequest{}, invalidFieldError("rule_ids", "Batch request must include at least one rule_id.", nil)
	}
	request.RuleIDs = ruleIDs
	return request, nil
}

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeTags(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > 64 || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func validProxyProtocol(value string) bool {
	return value == "" || value == "NONE" || value == "V1" || value == "V2"
}

func normalizedProxyProtocol(value string) string {
	if value == "" || value == "NONE" {
		return ""
	}
	return value
}
