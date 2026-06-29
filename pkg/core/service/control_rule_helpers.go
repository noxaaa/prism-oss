package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func simulatedEnabledRule(identity InternalIdentity, input RuleMutationInput, ruleID string, now string) repo.RuleRecord {
	binding := inboundBindingForRule(identity.OrganizationID, input, now)
	return repo.RuleRecord{
		ID:                  ruleID,
		OrganizationID:      identity.OrganizationID,
		OwnerUserID:         identity.UserID,
		Name:                input.Name,
		Enabled:             true,
		Status:              "ENABLED",
		FailurePolicy:       defaultFailurePolicy(input.FailurePolicy),
		DataplanePreference: defaultDataplanePreference(input.DataplanePreference),
		ForwardingType:      defaultForwardingType(input.ForwardingType),
		Protocol:            input.Protocol,
		MatchType:           input.Match.Type,
		InboundBindingID:    binding.ID,
		SNIHostname:         input.Match.SNIHostname,
		TargetType:          input.Upstream.Type,
		TargetID:            input.Upstream.TargetID,
		TargetGroupID:       input.Upstream.TargetGroupID,
		ProxyProtocolIn:     defaultProxyProtocol(input.ProxyProtocol.In),
		ProxyProtocolOut:    defaultProxyProtocol(input.ProxyProtocol.Out),
		Binding:             binding,
	}
}

func nodesInGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string) ([]repo.NodeRecord, error) {
	nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	result := make([]repo.NodeRecord, 0)
	for _, node := range nodes {
		for _, groupID := range node.GroupIDs {
			if groupID == nodeGroupID {
				result = append(result, node)
				break
			}
		}
	}
	return result, nil
}

func ensureNoRulesForNodeGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string) error {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.Binding.NodeGroupID == nodeGroupID {
			return ErrConflict
		}
	}
	return nil
}

func validateEnabledRulesForNodeSet(ctx context.Context, repositories repo.Repositories, organizationID string, nodes []repo.NodeRecord) error {
	rules, err := repositories.Rules().ListEnabledInboundBindings(ctx, organizationID)
	if err != nil {
		return err
	}
	seenBindings := make([]domain.InboundBinding, 0)
	for _, rule := range rules {
		groupNodes := nodesInGroupFromSet(nodes, rule.Binding.NodeGroupID)
		if !nodesCoverListenIPAndPortSegments(groupNodes, rule.Binding.ListenIP, rule.Protocol, rulePortSegments(rule.Binding)) {
			return &controlServiceError{
				Code:    "NODE_RULE_COVERAGE_CONFLICT",
				Message: "The node configuration does not cover an enabled rule listener.",
				Details: map[string]any{
					"node_group_id": rule.Binding.NodeGroupID,
					"rule_id":       rule.ID,
					"rule_name":     rule.Name,
					"listen_ip":     rule.Binding.ListenIP,
					"protocol":      rule.Protocol,
					"port":          rule.Binding.Port,
				},
				Cause: ErrConflict,
			}
		}
		if !nodesShareSendIP(groupNodes, rule.SendIP) {
			return &controlServiceError{
				Code:    "NODE_RULE_SEND_IP_CONFLICT",
				Message: "The node configuration does not allow the send IP required by an enabled rule.",
				Details: map[string]any{
					"node_group_id": rule.Binding.NodeGroupID,
					"rule_id":       rule.ID,
					"rule_name":     rule.Name,
					"send_ip":       rule.SendIP,
				},
				Cause: ErrConflict,
			}
		}
		ruleBindings := bindingsForNodes(groupNodes, rule.Binding.NodeGroupID, rule.Binding.ListenIP, rule.Protocol, rulePortSegments(rule.Binding), rule.MatchType, rule.SNIHostname, rule.ProxyProtocolIn, rule.ID)
		for _, binding := range ruleBindings {
			if err := domain.ValidateInboundBindingConflict(seenBindings, binding); err != nil {
				return err
			}
		}
		seenBindings = append(seenBindings, ruleBindings...)
	}
	return nil
}

func nodesInGroupFromSet(nodes []repo.NodeRecord, nodeGroupID string) []repo.NodeRecord {
	result := make([]repo.NodeRecord, 0)
	for _, node := range nodes {
		for _, groupID := range node.GroupIDs {
			if groupID == nodeGroupID {
				result = append(result, node)
				break
			}
		}
	}
	return result
}

func replaceNodeInSet(nodes []repo.NodeRecord, replacement repo.NodeRecord) []repo.NodeRecord {
	result := make([]repo.NodeRecord, 0, len(nodes))
	replaced := false
	for _, node := range nodes {
		if node.ID == replacement.ID {
			result = append(result, replacement)
			replaced = true
			continue
		}
		result = append(result, node)
	}
	if !replaced {
		result = append(result, replacement)
	}
	return result
}

func removeNodeFromSet(nodes []repo.NodeRecord, nodeID string) []repo.NodeRecord {
	result := make([]repo.NodeRecord, 0, len(nodes))
	for _, node := range nodes {
		if node.ID != nodeID {
			result = append(result, node)
		}
	}
	return result
}

func listenIPOptionsForNodes(nodes []repo.NodeRecord, protocol string, segments []repo.InboundBindingPortSegmentRecord) []ResourceOption {
	counts := map[string]int{}
	for _, node := range nodes {
		for _, listenIP := range node.ListenIPs {
			if !listenIP.Enabled {
				continue
			}
			if protocol != "" && len(segments) > 0 && !nodeCoversPortSegments(node, protocol, segments) {
				continue
			}
			counts[listenIP.ListenIP]++
		}
	}
	options := make([]ResourceOption, 0)
	for listenIP, count := range counts {
		if count == len(nodes) && count > 0 {
			options = append(options, ResourceOption{Value: listenIP, Label: fmt.Sprintf("%s (%d nodes)", listenIP, count)})
		}
	}
	sort.SliceStable(options, func(i int, j int) bool { return options[i].Value < options[j].Value })
	return options
}

func sendIPOptionsForNodes(nodes []repo.NodeRecord) []ResourceOption {
	counts := map[string]int{}
	labels := map[string]string{}
	for _, node := range nodes {
		seenForNode := map[string]struct{}{}
		for _, sendIP := range node.SendIPs {
			if !sendIP.Enabled {
				continue
			}
			if _, seen := seenForNode[sendIP.SendIP]; seen {
				continue
			}
			seenForNode[sendIP.SendIP] = struct{}{}
			counts[sendIP.SendIP]++
			if labels[sendIP.SendIP] == "" {
				labels[sendIP.SendIP] = sendIP.DisplayName
			}
		}
	}
	options := make([]ResourceOption, 0)
	for sendIP, count := range counts {
		if count != len(nodes) || count == 0 {
			continue
		}
		label := labels[sendIP]
		if label == "" || label == sendIP {
			label = fmt.Sprintf("%s (%d nodes)", sendIP, count)
		} else {
			label = fmt.Sprintf("%s - %s (%d nodes)", label, sendIP, count)
		}
		options = append(options, ResourceOption{Value: sendIP, Label: label})
	}
	sort.SliceStable(options, func(i int, j int) bool { return options[i].Value < options[j].Value })
	return options
}

func nodesCoverListenIPAndPortSegments(nodes []repo.NodeRecord, listenIP string, protocol string, segments []repo.InboundBindingPortSegmentRecord) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		hasListenIP := false
		for _, candidate := range node.ListenIPs {
			if candidate.Enabled && candidate.ListenIP == listenIP {
				hasListenIP = true
				break
			}
		}
		if !hasListenIP || !nodeCoversPortSegments(node, protocol, segments) || expandedPortCount(segments) > defaultMaxRulePorts(node.MaxRulePorts) {
			return false
		}
	}
	return true
}

func nodesShareListenIP(nodes []repo.NodeRecord, listenIP string) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		hasListenIP := false
		for _, candidate := range node.ListenIPs {
			if candidate.Enabled && candidate.ListenIP == listenIP {
				hasListenIP = true
				break
			}
		}
		if !hasListenIP {
			return false
		}
	}
	return true
}

func nodeCoversPort(node repo.NodeRecord, protocol string, port int) bool {
	if protocol == string(domain.ProtocolTCPUDP) {
		return nodeCoversPort(node, string(domain.ProtocolTCP), port) && nodeCoversPort(node, string(domain.ProtocolUDP), port)
	}
	for _, portRange := range node.PortRanges {
		if portRange.Enabled && portRange.Protocol == protocol && port >= portRange.StartPort && port <= portRange.EndPort {
			return true
		}
	}
	return false
}

func nodeCoversPortSegments(node repo.NodeRecord, protocol string, segments []repo.InboundBindingPortSegmentRecord) bool {
	for _, segment := range segments {
		for port := segment.StartPort; port <= segment.EndPort; port++ {
			if !nodeCoversPort(node, protocol, port) {
				return false
			}
		}
	}
	return true
}

func expandedPortCount(segments []repo.InboundBindingPortSegmentRecord) int {
	count := 0
	for _, segment := range segments {
		count += segment.EndPort - segment.StartPort + 1
	}
	return count
}

func nodesShareSendIP(nodes []repo.NodeRecord, sendIP string) bool {
	if strings.TrimSpace(sendIP) == "" {
		return true
	}
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		found := false
		for _, candidate := range node.SendIPs {
			if candidate.Enabled && candidate.SendIP == sendIP {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func inboundBindingForRule(organizationID string, input RuleMutationInput, now string) repo.InboundBindingRecord {
	segments := toInboundBindingPortSegments(organizationID, "", input.PortSegments, now)
	idSource := organizationID + "|" + input.NodeGroupID + "|" + input.ListenIP + "|" + input.Protocol + "|" + portSegmentsKey(segments) + "|" + input.Match.Type
	bindingID := uuid.NewHash(sha256.New(), uuid.Nil, []byte(idSource), 5).String()
	for index := range segments {
		segments[index].OrganizationID = organizationID
		segments[index].InboundBindingID = bindingID
	}
	return repo.InboundBindingRecord{
		ID:             uuid.NewHash(sha256.New(), uuid.Nil, []byte(idSource), 5).String(),
		OrganizationID: organizationID,
		NodeGroupID:    input.NodeGroupID,
		ListenIP:       input.ListenIP,
		Protocol:       input.Protocol,
		Port:           input.Port,
		PortSegments:   segments,
		MatchType:      input.Match.Type,
		CreatedAt:      now,
	}
}

func toInboundBindingPortSegments(organizationID string, bindingID string, inputs []RulePortSegmentInput, now string) []repo.InboundBindingPortSegmentRecord {
	segments := make([]repo.InboundBindingPortSegmentRecord, 0, len(inputs))
	for _, input := range inputs {
		segments = append(segments, repo.InboundBindingPortSegmentRecord{OrganizationID: organizationID, InboundBindingID: bindingID, StartPort: input.StartPort, EndPort: input.EndPort, CreatedAt: now})
	}
	return segments
}

func portSegmentsKey(segments []repo.InboundBindingPortSegmentRecord) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		parts = append(parts, strconv.Itoa(segment.StartPort)+"-"+strconv.Itoa(segment.EndPort))
	}
	return strings.Join(parts, ",")
}

func rulePortSegments(binding repo.InboundBindingRecord) []repo.InboundBindingPortSegmentRecord {
	if len(binding.PortSegments) == 0 {
		return []repo.InboundBindingPortSegmentRecord{{StartPort: binding.Port, EndPort: binding.Port}}
	}
	return binding.PortSegments
}

func toRulePortSegmentInputs(segments []repo.InboundBindingPortSegmentRecord) []RulePortSegmentInput {
	result := make([]RulePortSegmentInput, 0, len(segments))
	for _, segment := range segments {
		result = append(result, RulePortSegmentInput{StartPort: segment.StartPort, EndPort: segment.EndPort})
	}
	return result
}

func toRulePortSegmentInputsFromPayload(segments []RulePortSegmentPayload) []RulePortSegmentInput {
	result := make([]RulePortSegmentInput, 0, len(segments))
	for _, segment := range segments {
		result = append(result, RulePortSegmentInput(segment))
	}
	return result
}

func toRulePortSegmentPayloads(segments []repo.InboundBindingPortSegmentRecord) []RulePortSegmentPayload {
	result := make([]RulePortSegmentPayload, 0, len(segments))
	for _, segment := range segments {
		result = append(result, RulePortSegmentPayload{StartPort: segment.StartPort, EndPort: segment.EndPort})
	}
	return result
}

func defaultProxyProtocol(value string) string {
	if value == "" {
		return "NONE"
	}
	return value
}

func defaultForwardingType(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return string(domain.ForwardingTypeDirect)
	}
	return value
}

func validateRuleForwardingType(value string) error {
	if defaultForwardingType(value) != string(domain.ForwardingTypeDirect) {
		return validationFieldError("forwarding_type", "Only DIRECT forwarding is supported by the current runtime.", map[string]any{
			"actual": value,
		})
	}
	return nil
}

func validateRuleMatchType(value string) error {
	switch domain.MatchType(value) {
	case domain.MatchTypeAnyInbound, domain.MatchTypeTLSSNI:
		return nil
	default:
		return validationFieldError("match.type", "Unsupported rule match type.", map[string]any{
			"actual": value,
		})
	}
}

func inputFromRule(rule repo.RuleRecord) RuleMutationInput {
	return RuleMutationInput{
		Name:                rule.Name,
		Tags:                rule.Tags,
		NodeGroupID:         rule.Binding.NodeGroupID,
		ListenIP:            rule.Binding.ListenIP,
		SendIP:              rule.SendIP,
		FailurePolicy:       defaultFailurePolicy(rule.FailurePolicy),
		DataplanePreference: defaultDataplanePreference(rule.DataplanePreference),
		ForwardingType:      defaultForwardingType(rule.ForwardingType),
		Protocol:            rule.Protocol,
		Port:                rule.Binding.Port,
		PortSegments:        toRulePortSegmentInputs(rulePortSegments(rule.Binding)),
		Match:               RuleMatchInput{Type: rule.MatchType, SNIHostname: rule.SNIHostname},
		ProxyProtocol: RuleProxyProtocolInput{
			In:  rule.ProxyProtocolIn,
			Out: rule.ProxyProtocolOut,
		},
		Upstream: RuleUpstreamInput{Type: rule.TargetType, TargetID: rule.TargetID, TargetGroupID: rule.TargetGroupID},
		Enabled:  rule.Enabled,
	}
}

func ruleInputFromPortablePayload(rule PortableRulePayload, entry RuleImportEntry, targetIDsByRef map[string]string, targetGroupIDsByRef map[string]string) (RuleMutationInput, error) {
	upstreamType := strings.ToUpper(strings.TrimSpace(rule.Upstream.Type))
	upstream := RuleUpstreamInput{Type: upstreamType}
	switch upstreamType {
	case "TARGET":
		targetRef := strings.TrimSpace(rule.Upstream.TargetRef)
		targetID, ok := targetIDsByRef[targetRef]
		if targetRef == "" || !ok || strings.TrimSpace(rule.Upstream.TargetGroupRef) != "" {
			return RuleMutationInput{}, validationFieldError("upstream.target_ref", "Imported rule references an unresolved target.", map[string]any{
				"target_ref":       targetRef,
				"target_group_ref": strings.TrimSpace(rule.Upstream.TargetGroupRef),
			})
		}
		upstream.TargetID = targetID
	case "TARGET_GROUP":
		targetGroupRef := strings.TrimSpace(rule.Upstream.TargetGroupRef)
		targetGroupID, ok := targetGroupIDsByRef[targetGroupRef]
		if targetGroupRef == "" || !ok || strings.TrimSpace(rule.Upstream.TargetRef) != "" {
			return RuleMutationInput{}, validationFieldError("upstream.target_group_ref", "Imported rule references an unresolved target group.", map[string]any{
				"target_ref":       strings.TrimSpace(rule.Upstream.TargetRef),
				"target_group_ref": targetGroupRef,
			})
		}
		upstream.TargetGroupID = targetGroupID
	default:
		return RuleMutationInput{}, validationFieldError("upstream.type", "Imported rule has an unsupported upstream type.", map[string]any{
			"actual": upstreamType,
		})
	}

	input := RuleMutationInput{
		Name:                rule.Name,
		Tags:                append([]string{}, rule.Tags...),
		NodeGroupID:         entry.NodeGroupID,
		ListenIP:            entry.ListenIP,
		SendIP:              rule.SendIP,
		FailurePolicy:       rule.FailurePolicy,
		DataplanePreference: rule.DataplanePreference,
		ForwardingType:      rule.ForwardingType,
		Protocol:            rule.Protocol,
		Port:                rule.Port,
		PortSegments:        toRulePortSegmentInputsFromPayload(rule.PortSegments),
		Match:               RuleMatchInput{Type: rule.Match.Type, SNIHostname: rule.Match.SNIHostname},
		ProxyProtocol: RuleProxyProtocolInput{
			In:  rule.ProxyProtocol.In,
			Out: rule.ProxyProtocol.Out,
		},
		Upstream: upstream,
		Enabled:  true,
	}
	return validateRuleMutationInput(input)
}

func validateRuleMutationInput(input RuleMutationInput) (RuleMutationInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.NodeGroupID = strings.TrimSpace(input.NodeGroupID)
	input.ListenIP = strings.TrimSpace(input.ListenIP)
	input.SendIP = strings.TrimSpace(input.SendIP)
	input.FailurePolicy = defaultFailurePolicy(input.FailurePolicy)
	input.DataplanePreference = strings.ToUpper(strings.TrimSpace(input.DataplanePreference))
	if input.DataplanePreference == "" {
		input.DataplanePreference = DataplanePreferenceAuto
	}
	input.ForwardingType = defaultForwardingType(input.ForwardingType)
	input.Protocol = strings.ToUpper(strings.TrimSpace(input.Protocol))
	input.Match.Type = strings.ToUpper(strings.TrimSpace(input.Match.Type))
	input.Match.SNIHostname = strings.ToLower(strings.TrimSpace(input.Match.SNIHostname))
	input.ProxyProtocol.In = strings.ToUpper(strings.TrimSpace(input.ProxyProtocol.In))
	input.ProxyProtocol.Out = strings.ToUpper(strings.TrimSpace(input.ProxyProtocol.Out))
	input.Upstream.Type = strings.ToUpper(strings.TrimSpace(input.Upstream.Type))
	input.Upstream.TargetID = strings.TrimSpace(input.Upstream.TargetID)
	input.Upstream.TargetGroupID = strings.TrimSpace(input.Upstream.TargetGroupID)
	input.Tags = normalizeRuleTags(input.Tags)
	if input.Name == "" {
		return RuleMutationInput{}, validationFieldError("name", "Rule name is required.", nil)
	}
	if len(input.Name) > 120 {
		return RuleMutationInput{}, validationFieldError("name", "Rule name must be at most 120 characters.", map[string]any{
			"max_length": 120,
		})
	}
	if input.NodeGroupID == "" {
		return RuleMutationInput{}, validationFieldError("node_group_id", "Rule node_group_id is required.", nil)
	}
	if input.ListenIP == "" {
		return RuleMutationInput{}, validationFieldError("listen_ip", "Rule listen_ip is required.", nil)
	}
	input.PortSegments = normalizeRulePortSegments(input.Port, input.PortSegments)
	if len(input.PortSegments) == 0 {
		return RuleMutationInput{}, validationFieldError("port", "Rule port must be between 1 and 65535.", map[string]any{"actual": input.Port, "min": 1, "max": 65535})
	}
	input.Port = input.PortSegments[0].StartPort
	if input.SendIP != "" && (strings.Contains(input.SendIP, "/") || net.ParseIP(input.SendIP) == nil) {
		return RuleMutationInput{}, validationFieldError("send_ip", "Rule send_ip must be a literal IP address.", nil)
	}
	if err := validateRuleForwardingType(input.ForwardingType); err != nil {
		return RuleMutationInput{}, err
	}
	if err := validateFailurePolicy(input.FailurePolicy); err != nil {
		return RuleMutationInput{}, err
	}
	if err := validateDataplanePreference(input.DataplanePreference); err != nil {
		return RuleMutationInput{}, err
	}
	if input.Protocol != "TCP" && input.Protocol != "UDP" && input.Protocol != "TCP_UDP" {
		return RuleMutationInput{}, validationFieldError("protocol", "Rule protocol must be TCP, UDP, or TCP_UDP.", map[string]any{
			"actual": input.Protocol,
		})
	}
	if input.Match.Type != "ANY_INBOUND" && input.Match.Type != "TLS_SNI" {
		return RuleMutationInput{}, validationFieldError("match.type", "Rule match type must be ANY_INBOUND or TLS_SNI.", map[string]any{
			"actual": input.Match.Type,
		})
	}
	if input.Protocol != "TCP" && input.Match.Type != "ANY_INBOUND" {
		return RuleMutationInput{}, validationFieldError("match.type", "Only TCP rules can use TLS_SNI matching.", map[string]any{
			"match_type": input.Match.Type,
			"protocol":   input.Protocol,
		})
	}
	if input.Match.Type == "TLS_SNI" && input.Match.SNIHostname == "" {
		return RuleMutationInput{}, validationFieldError("match.sni_hostname", "TLS_SNI rules require an SNI hostname.", nil)
	}
	if input.Match.Type == "ANY_INBOUND" {
		input.Match.SNIHostname = ""
	}
	if !validRuleProxyProtocol(input.ProxyProtocol.In) || !validRuleProxyProtocol(input.ProxyProtocol.Out) {
		return RuleMutationInput{}, validationError("Rule proxy protocol must be NONE, V1, or V2.", map[string]any{
			"proxy_protocol.in":  input.ProxyProtocol.In,
			"proxy_protocol.out": input.ProxyProtocol.Out,
		})
	}
	if input.Protocol == "UDP" && (normalizedRuleProxyProtocol(input.ProxyProtocol.In) != "" || normalizedRuleProxyProtocol(input.ProxyProtocol.Out) != "") {
		return RuleMutationInput{}, validationError("UDP rules cannot use Proxy Protocol.", map[string]any{
			"protocol":           input.Protocol,
			"proxy_protocol.in":  input.ProxyProtocol.In,
			"proxy_protocol.out": input.ProxyProtocol.Out,
		})
	}
	switch input.Upstream.Type {
	case "TARGET":
		if input.Upstream.TargetID == "" || input.Upstream.TargetGroupID != "" {
			return RuleMutationInput{}, validationError("TARGET upstream requires target_id and no target_group_id.", map[string]any{
				"upstream.target_id_present":       input.Upstream.TargetID != "",
				"upstream.target_group_id_present": input.Upstream.TargetGroupID != "",
			})
		}
	case "TARGET_GROUP":
		if input.Upstream.TargetGroupID == "" || input.Upstream.TargetID != "" {
			return RuleMutationInput{}, validationError("TARGET_GROUP upstream requires target_group_id and no target_id.", map[string]any{
				"upstream.target_id_present":       input.Upstream.TargetID != "",
				"upstream.target_group_id_present": input.Upstream.TargetGroupID != "",
			})
		}
	default:
		return RuleMutationInput{}, validationFieldError("upstream.type", "Rule upstream type must be TARGET or TARGET_GROUP.", map[string]any{
			"actual": input.Upstream.Type,
		})
	}
	return input, nil
}

func toPortableRulePayload(rule repo.RuleRecord, targetRefsByID map[string]string, targetGroupRefsByID map[string]string) PortableRulePayload {
	upstream := PortableRuleUpstreamPayload{Type: rule.TargetType}
	if rule.TargetType == "TARGET" {
		upstream.TargetRef = targetRefsByID[rule.TargetID]
	}
	if rule.TargetType == "TARGET_GROUP" {
		upstream.TargetGroupRef = targetGroupRefsByID[rule.TargetGroupID]
	}
	return PortableRulePayload{
		Name:                rule.Name,
		Tags:                append([]string{}, rule.Tags...),
		FailurePolicy:       defaultFailurePolicy(rule.FailurePolicy),
		DataplanePreference: defaultDataplanePreference(rule.DataplanePreference),
		ForwardingType:      defaultForwardingType(rule.ForwardingType),
		Protocol:            rule.Protocol,
		Port:                rule.Binding.Port,
		PortSegments:        toRulePortSegmentPayloads(rulePortSegments(rule.Binding)),
		SendIP:              rule.SendIP,
		Match:               RuleMatchPayload{Type: rule.MatchType, SNIHostname: rule.SNIHostname},
		ProxyProtocol: RuleProxyProtocolInput{
			In:  rule.ProxyProtocolIn,
			Out: rule.ProxyProtocolOut,
		},
		Upstream: upstream,
	}
}

func toPortableTargetPayload(target repo.TargetRecord, ref string) PortableTargetPayload {
	return PortableTargetPayload{Ref: ref, Name: target.Name, Host: target.Host, Port: target.Port, Enabled: target.Enabled}
}

func toPortableTargetGroupPayload(group repo.TargetGroupRecord, ref string, targetRefsByID map[string]string) PortableTargetGroupPayload {
	members := make([]PortableTargetGroupMemberPayload, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, PortableTargetGroupMemberPayload{TargetRef: targetRefsByID[member.TargetID], Priority: member.Priority, Enabled: member.Enabled})
	}
	return PortableTargetGroupPayload{
		Ref:         ref,
		Name:        group.Name,
		Description: group.Description,
		Scheduler:   group.Scheduler,
		Members:     members,
	}
}

func portableRef(prefix string, index int) string {
	return fmt.Sprintf("%s_%d", prefix, index+1)
}

func normalizeRuleTags(values []string) []string {
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

func normalizeRulePortSegments(port int, values []RulePortSegmentInput) []RulePortSegmentInput {
	if len(values) == 0 {
		if port < 1 || port > 65535 {
			return nil
		}
		return []RulePortSegmentInput{{StartPort: port, EndPort: port}}
	}
	segments := make([]RulePortSegmentInput, 0, len(values))
	for _, value := range values {
		if value.EndPort == 0 {
			value.EndPort = value.StartPort
		}
		if value.StartPort < 1 || value.StartPort > 65535 || value.EndPort < 1 || value.EndPort > 65535 || value.StartPort > value.EndPort {
			return nil
		}
		segments = append(segments, value)
	}
	sort.Slice(segments, func(i int, j int) bool {
		if segments[i].StartPort == segments[j].StartPort {
			return segments[i].EndPort < segments[j].EndPort
		}
		return segments[i].StartPort < segments[j].StartPort
	})
	result := make([]RulePortSegmentInput, 0, len(segments))
	for _, segment := range segments {
		if len(result) == 0 || segment.StartPort > result[len(result)-1].EndPort+1 {
			result = append(result, segment)
			continue
		}
		if segment.EndPort > result[len(result)-1].EndPort {
			result[len(result)-1].EndPort = segment.EndPort
		}
	}
	return result
}

func validRuleProxyProtocol(value string) bool {
	return value == "" || value == "NONE" || value == "V1" || value == "V2"
}

func normalizedRuleProxyProtocol(value string) string {
	if value == "" || value == "NONE" {
		return ""
	}
	return value
}

func bumpDesiredConfigForNodeGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string, now string) error {
	if nodeGroupID == "" {
		return nil
	}
	return repositories.Nodes().IncrementDesiredConfigForNodeGroup(ctx, organizationID, nodeGroupID, now)
}

func bumpDesiredConfigForRulesUsingTarget(ctx context.Context, repositories repo.Repositories, organizationID string, targetID string, now string) error {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	groupsUsingTarget := make(map[string]struct{})
	for _, group := range targetGroups {
		for _, member := range group.Members {
			if member.TargetID == targetID {
				groupsUsingTarget[group.ID] = struct{}{}
				break
			}
		}
	}
	nodeGroups := make(map[string]struct{})
	affectedRules := make([]repo.RuleRecord, 0)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		affected := false
		if rule.TargetType == "TARGET" && rule.TargetID == targetID {
			affected = true
		}
		if rule.TargetType == "TARGET_GROUP" {
			if _, ok := groupsUsingTarget[rule.TargetGroupID]; ok {
				affected = true
			}
		}
		if !affected {
			continue
		}
		nodeGroups[rule.Binding.NodeGroupID] = struct{}{}
		affectedRules = append(affectedRules, rule)
	}
	if err := bumpDesiredConfigForNodeGroups(ctx, repositories, organizationID, nodeGroups, now); err != nil {
		return err
	}
	for _, rule := range affectedRules {
		if err := syncRuleDeploymentPending(ctx, repositories, organizationID, rule, now, uuid.NewString); err != nil {
			return err
		}
	}
	return nil
}

func bumpDesiredConfigForRulesUsingTargetGroup(ctx context.Context, repositories repo.Repositories, organizationID string, targetGroupID string, now string) error {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	nodeGroups := make(map[string]struct{})
	affectedRules := make([]repo.RuleRecord, 0)
	for _, rule := range rules {
		if rule.Enabled && rule.TargetType == "TARGET_GROUP" && rule.TargetGroupID == targetGroupID {
			nodeGroups[rule.Binding.NodeGroupID] = struct{}{}
			affectedRules = append(affectedRules, rule)
		}
	}
	if err := bumpDesiredConfigForNodeGroups(ctx, repositories, organizationID, nodeGroups, now); err != nil {
		return err
	}
	for _, rule := range affectedRules {
		if err := syncRuleDeploymentPending(ctx, repositories, organizationID, rule, now, uuid.NewString); err != nil {
			return err
		}
	}
	return nil
}

func bumpDesiredConfigForNodeGroups(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroups map[string]struct{}, now string) error {
	for nodeGroupID := range nodeGroups {
		if err := bumpDesiredConfigForNodeGroup(ctx, repositories, organizationID, nodeGroupID, now); err != nil {
			return err
		}
	}
	return nil
}

func defaultRuleCopyName(sourceName string) string {
	const maxNameBytes = 120
	const suffix = " copy"
	if len(sourceName)+len(suffix) <= maxNameBytes {
		return sourceName + suffix
	}
	limit := maxNameBytes - len(suffix)
	prefixBytes := 0
	for index, currentRune := range sourceName {
		runeBytes := len(string(currentRune))
		if prefixBytes+runeBytes > limit {
			return sourceName[:index] + suffix
		}
		prefixBytes += runeBytes
	}
	return sourceName + suffix
}

func (service *ControlService) toRulePayloadForIdentity(identity InternalIdentity, rule repo.RuleRecord, nodes []repo.NodeRecord, deployments []repo.RuleDeploymentRecord) RulePayload {
	return toRulePayloadWithDeploymentDetails(rule, nodes, deployments, service.canReadRuleDeploymentNodes(identity, rule.Binding.NodeGroupID))
}

func (service *ControlService) canReadRuleDeploymentNodes(identity InternalIdentity, nodeGroupID string) bool {
	if !service.canUseNodeGroup(identity, nodeGroupID) {
		return false
	}
	return service.hasPermission(identity, string(domain.PermissionNodesRead)) ||
		service.hasPermission(identity, string(domain.PermissionNodesManage))
}

func toRulePayloadWithDeploymentDetails(rule repo.RuleRecord, nodes []repo.NodeRecord, deployments []repo.RuleDeploymentRecord, includeDeploymentNodeDetails bool) RulePayload {
	descriptions := make([]string, 0)
	for _, node := range nodes {
		if strings.TrimSpace(node.PublicDescription) != "" {
			descriptions = append(descriptions, node.PublicDescription)
		}
	}
	sort.Strings(descriptions)
	return RulePayload{
		ID:                  rule.ID,
		Name:                rule.Name,
		Status:              rule.Status,
		Enabled:             rule.Enabled,
		Tags:                append([]string{}, rule.Tags...),
		NodeGroupID:         rule.Binding.NodeGroupID,
		ListenIP:            rule.Binding.ListenIP,
		SendIP:              rule.SendIP,
		FailurePolicy:       defaultFailurePolicy(rule.FailurePolicy),
		DataplanePreference: defaultDataplanePreference(rule.DataplanePreference),
		ForwardingType:      defaultForwardingType(rule.ForwardingType),
		Protocol:            rule.Protocol,
		Port:                rule.Binding.Port,
		PortSegments:        toRulePortSegmentPayloads(rulePortSegments(rule.Binding)),
		Match:               RuleMatchPayload{Type: rule.MatchType, SNIHostname: rule.SNIHostname},
		ProxyProtocol: RuleProxyProtocolInput{
			In:  rule.ProxyProtocolIn,
			Out: rule.ProxyProtocolOut,
		},
		Upstream:      RuleUpstreamInput{Type: rule.TargetType, TargetID: rule.TargetID, TargetGroupID: rule.TargetGroupID},
		OwnerUserID:   rule.OwnerUserID,
		ConfigVersion: rule.ConfigVersion,
		ConnectInfo:   RuleConnectInfoPayload{Protocol: rule.Protocol, ListenPort: rule.Binding.Port, ListenIP: rule.Binding.ListenIP, NodeDescriptions: descriptions},
		Deployment:    ruleDeploymentPayload(rule, nodes, deployments, includeDeploymentNodeDetails),
	}
}

func (service *ControlService) canReadRule(identity InternalIdentity, rule repo.RuleRecord) bool {
	if !service.canUseNodeGroup(identity, rule.Binding.NodeGroupID) {
		return false
	}
	return service.hasPermission(identity, string(domain.PermissionRulesReadAll)) ||
		service.hasPermission(identity, string(domain.PermissionRulesManageAll)) ||
		((service.hasPermission(identity, string(domain.PermissionRulesReadOwn)) ||
			service.hasPermission(identity, string(domain.PermissionRulesManageOwn))) && rule.OwnerUserID == identity.UserID)
}

func (service *ControlService) hasRuleReadOrManagePermission(identity InternalIdentity) bool {
	return service.hasPermission(identity, string(domain.PermissionRulesReadOwn)) ||
		service.hasPermission(identity, string(domain.PermissionRulesReadAll)) ||
		service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) ||
		service.hasPermission(identity, string(domain.PermissionRulesManageAll))
}

func (service *ControlService) canManageRule(identity InternalIdentity, rule repo.RuleRecord) bool {
	if !service.canUseNodeGroup(identity, rule.Binding.NodeGroupID) {
		return false
	}
	return service.hasPermission(identity, string(domain.PermissionRulesManageAll)) || (service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && rule.OwnerUserID == identity.UserID)
}
