package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

const (
	ruleImportFormatPortableExport = "PORTABLE_EXPORT"
	ruleImportFormatNyanpass       = "NYANPASS"
)

type ruleImportSourceResult struct {
	Payload RulesExportPayload
	Errors  []RuleImportIssue
	Skipped int
}

type ruleImportIssueError struct {
	Code    string
	Message string
	Details map[string]any
}

func (err *ruleImportIssueError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return err.Code
}

type nyanpassRulePayload struct {
	Name                string          `json:"name"`
	Dest                []string        `json:"dest"`
	DestPolicy          string          `json:"dest_policy"`
	ListenPort          int             `json:"listen_port"`
	AcceptProxyProtocol int             `json:"accept_proxy_protocol"`
	ProxyProtocol       int             `json:"proxy_protocol"`
	TLS                 json.RawMessage `json:"tls"`
	TLSPresent          bool            `json:"-"`
}

type parsedNyanpassDest struct {
	Host      string
	Port      int
	HostPort  string
	DestIndex int
}

func (rule *nyanpassRulePayload) UnmarshalJSON(data []byte) error {
	type alias nyanpassRulePayload
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, rule.TLSPresent = fields["tls"]
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*rule = nyanpassRulePayload(decoded)
	_, rule.TLSPresent = fields["tls"]
	return nil
}

func (service *ControlService) ruleImportSource(input RuleImportInput) (ruleImportSourceResult, error) {
	format := strings.ToUpper(strings.TrimSpace(input.Format))
	sourceText := strings.TrimSpace(input.SourceText)
	if format == "" {
		return ruleImportSourceResult{}, validationFieldError("format", "Import format is required.", map[string]any{
			"supported_formats": []string{ruleImportFormatPortableExport, ruleImportFormatNyanpass},
		})
	}
	if sourceText == "" {
		return ruleImportSourceResult{}, validationFieldError("source_text", "Import source_text is required.", nil)
	}
	switch format {
	case ruleImportFormatPortableExport:
		var payload RulesExportPayload
		if err := json.Unmarshal([]byte(sourceText), &payload); err != nil {
			return ruleImportSourceResult{}, validationFieldError("source_text", "Import source_text must be valid rules.export.v1 JSON.", map[string]any{
				"format": format,
				"error":  err.Error(),
			})
		}
		return ruleImportSourceResult{Payload: payload, Errors: []RuleImportIssue{}}, nil
	case ruleImportFormatNyanpass:
		return service.nyanpassImportSource(sourceText)
	default:
		return ruleImportSourceResult{}, validationFieldError("format", "Unsupported import format.", map[string]any{
			"actual":            format,
			"supported_formats": []string{ruleImportFormatPortableExport, ruleImportFormatNyanpass},
		})
	}
}

func (service *ControlService) nyanpassImportSource(sourceText string) (ruleImportSourceResult, error) {
	rules, err := parseNyanpassRules(sourceText)
	if err != nil {
		return ruleImportSourceResult{}, validationFieldError("source_text", "Import source_text must be valid Nyanpass JSON.", map[string]any{
			"format": ruleImportFormatNyanpass,
			"error":  err.Error(),
		})
	}
	payload := RulesExportPayload{
		SchemaVersion: "rules.export.v1",
		ExportedAt:    service.timestamp(),
		Rules:         []PortableRulePayload{},
		Targets:       []PortableTargetPayload{},
		TargetGroups:  []PortableTargetGroupPayload{},
	}
	result := ruleImportSourceResult{Payload: payload, Errors: []RuleImportIssue{}}
	targetRefsByHostPort := map[string]string{}
	for index, rule := range rules {
		portableRule, targets, group, err := nyanpassRuleToPortable(index, rule, targetRefsByHostPort)
		if err != nil {
			result.Errors = append(result.Errors, importIssueFromError("nyanpass", index, err))
			result.Skipped++
			continue
		}
		result.Payload.Targets = append(result.Payload.Targets, targets...)
		if group != nil {
			result.Payload.TargetGroups = append(result.Payload.TargetGroups, *group)
		}
		result.Payload.Rules = append(result.Payload.Rules, portableRule)
	}
	return result, nil
}

func parseNyanpassRules(sourceText string) ([]nyanpassRulePayload, error) {
	sourceText = strings.TrimSpace(sourceText)
	if sourceText == "" {
		return nil, ErrInvalidInput
	}
	if strings.HasPrefix(sourceText, "[") {
		var rules []nyanpassRulePayload
		if err := json.Unmarshal([]byte(sourceText), &rules); err != nil {
			return nil, err
		}
		return rules, nil
	}
	decoder := json.NewDecoder(strings.NewReader(sourceText))
	rules := []nyanpassRulePayload{}
	for {
		var rule nyanpassRulePayload
		if err := decoder.Decode(&rule); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func nyanpassRuleToPortable(index int, rule nyanpassRulePayload, targetRefsByHostPort map[string]string) (PortableRulePayload, []PortableTargetPayload, *PortableTargetGroupPayload, error) {
	name := strings.TrimSpace(rule.Name)
	destPolicy := strings.ToLower(strings.TrimSpace(rule.DestPolicy))
	if rule.TLSPresent {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_TLS_UNSUPPORTED", "nyanpass tls/origin fetch import is not supported by the current runtime", map[string]any{
			"format": ruleImportFormatNyanpass,
		})
	}
	if name == "" || len(name) > 120 {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_INVALID_NAME", "invalid nyanpass rule name", map[string]any{
			"max_length": 120,
		})
	}
	if rule.ListenPort < 1 || rule.ListenPort > 65535 {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_INVALID_LISTEN_PORT", "invalid nyanpass listen_port", map[string]any{
			"listen_port": rule.ListenPort,
			"min":         1,
			"max":         65535,
		})
	}
	if len(rule.Dest) == 0 {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_DEST_REQUIRED", "nyanpass dest is required", nil)
	}
	if destPolicy != "" && destPolicy != "ip_hash" {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_UNSUPPORTED_DEST_POLICY", "unsupported nyanpass dest_policy", map[string]any{
			"actual":           rule.DestPolicy,
			"supported_values": []string{"", "ip_hash"},
		})
	}
	proxyIn, err := nyanpassProxyProtocol(rule.AcceptProxyProtocol)
	if err != nil {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_INVALID_ACCEPT_PROXY_PROTOCOL", "invalid nyanpass accept_proxy_protocol", map[string]any{
			"actual":           rule.AcceptProxyProtocol,
			"supported_values": []int{0, 1, 2},
		})
	}
	proxyOut, err := nyanpassProxyProtocol(rule.ProxyProtocol)
	if err != nil {
		return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_INVALID_PROXY_PROTOCOL", "invalid nyanpass proxy_protocol", map[string]any{
			"actual":           rule.ProxyProtocol,
			"supported_values": []int{0, 1, 2},
		})
	}
	parsedDests := make([]parsedNyanpassDest, 0, len(rule.Dest))
	for destIndex, rawDest := range rule.Dest {
		host, port, err := parseNyanpassDest(rawDest)
		if err != nil {
			return PortableRulePayload{}, nil, nil, newRuleImportIssueError("IMPORT_NYANPASS_INVALID_DEST", "invalid nyanpass dest", map[string]any{
				"dest_index": destIndex,
				"dest":       rawDest,
				"expected":   "host:port",
				"min_port":   1,
				"max_port":   65535,
			})
		}
		parsedDests = append(parsedDests, parsedNyanpassDest{
			Host:      host,
			Port:      port,
			HostPort:  importTargetHostPortKey(host, port),
			DestIndex: destIndex,
		})
	}
	targets := make([]PortableTargetPayload, 0, len(parsedDests))
	members := make([]PortableTargetGroupMemberPayload, 0, len(parsedDests))
	targetRefs := make([]string, 0, len(parsedDests))
	pendingTargetRefsByHostPort := map[string]string{}
	for _, dest := range parsedDests {
		key := dest.HostPort
		targetRef, exists := targetRefsByHostPort[key]
		if !exists {
			targetRef, exists = pendingTargetRefsByHostPort[key]
			if !exists {
				targetRef = fmt.Sprintf("nyanpass_target_%d", len(targetRefsByHostPort)+len(pendingTargetRefsByHostPort)+1)
				pendingTargetRefsByHostPort[key] = targetRef
				targets = append(targets, PortableTargetPayload{
					Ref:     targetRef,
					Name:    boundedImportName(name, fmt.Sprintf("-target-%d", dest.DestIndex+1), 120),
					Host:    dest.Host,
					Port:    dest.Port,
					Enabled: true,
				})
			}
		}
		targetRefs = append(targetRefs, targetRef)
		members = append(members, PortableTargetGroupMemberPayload{TargetRef: targetRef, Priority: 10, Enabled: true})
	}
	for key, targetRef := range pendingTargetRefsByHostPort {
		targetRefsByHostPort[key] = targetRef
	}
	upstream := PortableRuleUpstreamPayload{Type: "TARGET", TargetRef: targetRefs[0]}
	var group *PortableTargetGroupPayload
	if len(targetRefs) > 1 {
		groupRef := fmt.Sprintf("nyanpass_group_%d", index+1)
		upstream = PortableRuleUpstreamPayload{Type: "TARGET_GROUP", TargetGroupRef: groupRef}
		group = &PortableTargetGroupPayload{
			Ref:         groupRef,
			Name:        boundedImportName(name, "-group", 120),
			Description: "Imported from Nyanpass",
			Scheduler:   targetGroupSchedulerPriorityIPHash,
			Members:     members,
		}
	}
	return PortableRulePayload{
		Name:           name,
		Tags:           []string{},
		ForwardingType: "DIRECT",
		Protocol:       "TCP",
		Port:           rule.ListenPort,
		Match:          RuleMatchPayload{Type: "ANY_INBOUND"},
		ProxyProtocol:  RuleProxyProtocolInput{In: proxyIn, Out: proxyOut},
		Upstream:       upstream,
	}, targets, group, nil
}

func newRuleImportIssueError(code string, message string, details map[string]any) error {
	return &ruleImportIssueError{Code: code, Message: message, Details: copyErrorDetails(details)}
}

func parseNyanpassDest(value string) (string, int, error) {
	host, rawPort, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, err
	}
	host = strings.TrimSpace(host)
	port, err := strconv.Atoi(rawPort)
	if host == "" || strings.ContainsAny(host, " \t\r\n") || port < 1 || port > 65535 || err != nil {
		return "", 0, ErrInvalidInput
	}
	return host, port, nil
}

func nyanpassProxyProtocol(value int) (string, error) {
	switch value {
	case 0:
		return "NONE", nil
	case 1:
		return "V1", nil
	case 2:
		return "V2", nil
	default:
		return "", ErrInvalidInput
	}
}

func boundedImportName(base string, suffix string, maxLen int) string {
	base = strings.TrimSpace(base)
	if maxLen <= 0 {
		return ""
	}
	if len(base)+len(suffix) <= maxLen {
		return base + suffix
	}
	if len(suffix) >= maxLen {
		return truncateUTF8ByBytes(suffix, maxLen)
	}
	return strings.TrimSpace(truncateUTF8ByBytes(base, maxLen-len(suffix))) + suffix
}

func truncateUTF8ByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > maxBytes {
			break
		}
		end = index
	}
	return value[:end]
}
