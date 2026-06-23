package validator

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid request")

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

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
	Name                string                  `json:"name"`
	GroupIDs            []string                `json:"group_ids"`
	ListenIPs           []NodeListenIP          `json:"listen_ips"`
	PortRanges          []NodePortRange         `json:"port_ranges"`
	DNSPublishAddresses []NodeDNSPublishAddress `json:"dns_publish_addresses"`
	PublicDescription   string                  `json:"public_description"`
}

type NodePatchRequest struct {
	Name                *string                  `json:"name"`
	GroupIDs            *[]string                `json:"group_ids"`
	ListenIPs           *[]NodeListenIP          `json:"listen_ips"`
	PortRanges          *[]NodePortRange         `json:"port_ranges"`
	DNSPublishAddresses *[]NodeDNSPublishAddress `json:"dns_publish_addresses"`
	PublicDescription   *string                  `json:"public_description"`
}

type NodeListenIP struct {
	ListenIP    string `json:"listen_ip"`
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
	Enabled  bool   `json:"enabled"`
}

type RuleRequest struct {
	Name           string               `json:"name"`
	Tags           []string             `json:"tags"`
	NodeGroupID    string               `json:"node_group_id"`
	ListenIP       string               `json:"listen_ip"`
	FailurePolicy  string               `json:"failure_policy"`
	ForwardingType string               `json:"forwarding_type"`
	Protocol       string               `json:"protocol"`
	Port           int                  `json:"port"`
	Match          RuleMatchRequest     `json:"match"`
	ProxyProtocol  ProxyProtocolRequest `json:"proxy_protocol"`
	Upstream       RuleUpstreamRequest  `json:"upstream"`
	Enabled        bool                 `json:"enabled"`
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
	request.GroupIDs = normalizeIDs(request.GroupIDs)
	if request.Name == "" || len(request.Name) > 120 || len(request.PublicDescription) > 2000 {
		return NodeRequest{}, ErrInvalidRequest
	}
	listenIPs, err := validateListenIPs(request.ListenIPs)
	if err != nil {
		return NodeRequest{}, err
	}
	portRanges, err := validatePortRanges(request.PortRanges)
	if err != nil {
		return NodeRequest{}, err
	}
	dnsPublishAddresses, err := validateDNSPublishAddresses(request.DNSPublishAddresses)
	if err != nil {
		return NodeRequest{}, err
	}
	request.ListenIPs = listenIPs
	request.PortRanges = portRanges
	request.DNSPublishAddresses = dnsPublishAddresses
	return request, nil
}

func ValidateNodePatchRequest(request NodePatchRequest) (NodePatchRequest, error) {
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || len(name) > 120 {
			return NodePatchRequest{}, ErrInvalidRequest
		}
		request.Name = &name
	}
	if request.PublicDescription != nil {
		description := strings.TrimSpace(*request.PublicDescription)
		if len(description) > 2000 {
			return NodePatchRequest{}, ErrInvalidRequest
		}
		request.PublicDescription = &description
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
	if request.PortRanges != nil {
		portRanges, err := validatePortRanges(*request.PortRanges)
		if err != nil {
			return NodePatchRequest{}, err
		}
		request.PortRanges = &portRanges
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
		seen[member.TargetID] = true
		request.Members[index] = member
	}
	return request, nil
}

func ValidateRuleRequest(request RuleRequest) (RuleRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.NodeGroupID = strings.TrimSpace(request.NodeGroupID)
	request.ListenIP = strings.TrimSpace(request.ListenIP)
	request.FailurePolicy = strings.ToUpper(strings.TrimSpace(request.FailurePolicy))
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
	if request.Port < 1 || request.Port > 65535 {
		return RuleRequest{}, invalidFieldError("port", "Rule port must be between 1 and 65535.", map[string]any{"actual": request.Port, "min": 1, "max": 65535})
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
	if request.Match.Type != "ANY_INBOUND" && request.Match.Type != "TLS_SNI" {
		return RuleRequest{}, invalidFieldError("match.type", "Rule match type must be ANY_INBOUND or TLS_SNI.", map[string]any{"actual": request.Match.Type})
	}
	if request.Protocol != "TCP" && request.Match.Type != "ANY_INBOUND" {
		return RuleRequest{}, invalidFieldError("match.type", "Only TCP rules can use TLS_SNI matching.", map[string]any{"match_type": request.Match.Type, "protocol": request.Protocol})
	}
	if request.Match.Type == "TLS_SNI" && request.Match.SNIHostname == "" {
		return RuleRequest{}, invalidFieldError("match.sni_hostname", "TLS_SNI rules require an SNI hostname.", nil)
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

func validateListenIPs(values []NodeListenIP) ([]NodeListenIP, error) {
	seen := make(map[string]bool)
	normalized := make([]NodeListenIP, 0, len(values))
	for _, value := range values {
		value.ListenIP = strings.TrimSpace(value.ListenIP)
		value.DisplayName = strings.TrimSpace(value.DisplayName)
		if value.ListenIP == "" {
			continue
		}
		if net.ParseIP(value.ListenIP) == nil || len(value.DisplayName) > 120 {
			return nil, ErrInvalidRequest
		}
		if seen[value.ListenIP] {
			return nil, ErrInvalidRequest
		}
		if value.DisplayName == "" {
			if value.ListenIP == "0.0.0.0" {
				value.DisplayName = "default"
			} else {
				value.DisplayName = value.ListenIP
			}
		}
		seen[value.ListenIP] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []NodeListenIP{{ListenIP: "0.0.0.0", DisplayName: "default"}}, nil
	}
	return normalized, nil
}

func validatePortRanges(values []NodePortRange) ([]NodePortRange, error) {
	normalized := make([]NodePortRange, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value.Protocol = strings.ToUpper(strings.TrimSpace(value.Protocol))
		if value.Protocol == "" {
			value.Protocol = "TCP"
		}
		if value.StartPort == 0 {
			value.StartPort = 10000
		}
		if value.EndPort == 0 {
			value.EndPort = 20000
		}
		if value.Protocol != "TCP" && value.Protocol != "UDP" {
			return nil, ErrInvalidRequest
		}
		if value.StartPort < 1 || value.StartPort > 65535 || value.EndPort < 1 || value.EndPort > 65535 || value.StartPort > value.EndPort {
			return nil, ErrInvalidRequest
		}
		key := value.Protocol + ":" + strconv.Itoa(value.StartPort) + ":" + strconv.Itoa(value.EndPort)
		if seen[key] {
			return nil, ErrInvalidRequest
		}
		seen[key] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []NodePortRange{{Protocol: "TCP", StartPort: 10000, EndPort: 20000}}, nil
	}
	return normalized, nil
}

func validateDNSPublishAddresses(values []NodeDNSPublishAddress) ([]NodeDNSPublishAddress, error) {
	normalized := make([]NodeDNSPublishAddress, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value.AddressType = strings.ToUpper(strings.TrimSpace(value.AddressType))
		value.Address = strings.TrimSpace(value.Address)
		if value.Address == "" {
			continue
		}
		ip := net.ParseIP(value.Address)
		if ip == nil || !isPublicIP(ip) {
			return nil, ErrInvalidRequest
		}
		if value.AddressType == "" {
			if ip.To4() == nil {
				value.AddressType = "AAAA"
			} else {
				value.AddressType = "A"
			}
		}
		if (value.AddressType == "A" && ip.To4() == nil) || (value.AddressType == "AAAA" && ip.To4() != nil) {
			return nil, ErrInvalidRequest
		}
		if value.AddressType != "A" && value.AddressType != "AAAA" {
			return nil, ErrInvalidRequest
		}
		key := value.AddressType + "\x00" + value.Address
		if seen[key] {
			return nil, ErrInvalidRequest
		}
		seen[key] = true
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() && !isCarrierGradeNATAddress(ip)
}

func isCarrierGradeNATAddress(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 0x40
}
