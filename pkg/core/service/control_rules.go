package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

var errImportDryRunRollback = errors.New("rule import dry-run rollback")

func (service *ControlService) NodeGroupListenIPOptions(ctx context.Context, identity InternalIdentity, nodeGroupID string, protocol string, port int) ([]ResourceOption, error) {
	if !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) && !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return nil, ErrForbidden
	}
	if !service.canUseNodeGroup(identity, nodeGroupID) {
		return []ResourceOption{}, nil
	}
	var result []ResourceOption
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, nodeGroupID)
		if err != nil {
			return err
		}
		result = listenIPOptionsForNodes(nodes, protocol, port)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateRule(ctx context.Context, identity InternalIdentity, input RuleMutationInput) (RulePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return RulePayload{}, ErrForbidden
	}
	input.ForwardingType = defaultForwardingType(input.ForwardingType)
	var result RulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := service.prepareRuleMutation(ctx, repositories, identity, "", input, input.Enabled); err != nil {
			return err
		}
		if err := ensureRuleLimitAvailable(ctx, repositories, identity, identity.UserID, 1); err != nil {
			return err
		}
		now := service.timestamp()
		status := "DISABLED"
		if input.Enabled {
			status = "ENABLED"
		}
		binding := inboundBindingForRule(identity.OrganizationID, input, now)
		rule := repo.RuleRecord{
			ID:               service.newID(),
			OrganizationID:   identity.OrganizationID,
			OwnerUserID:      identity.UserID,
			Name:             input.Name,
			Enabled:          input.Enabled,
			Status:           status,
			ForwardingType:   input.ForwardingType,
			Protocol:         input.Protocol,
			MatchType:        input.Match.Type,
			InboundBindingID: binding.ID,
			SNIHostname:      input.Match.SNIHostname,
			TargetType:       input.Upstream.Type,
			TargetID:         input.Upstream.TargetID,
			TargetGroupID:    input.Upstream.TargetGroupID,
			ProxyProtocolIn:  defaultProxyProtocol(input.ProxyProtocol.In),
			ProxyProtocolOut: defaultProxyProtocol(input.ProxyProtocol.Out),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := repositories.Rules().CreateRule(ctx, rule, binding, input.Tags, now, service.newID); err != nil {
			return err
		}
		if input.Enabled {
			if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, binding.NodeGroupID, now); err != nil {
				return err
			}
		}
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, rule.ID)
		if err != nil {
			return err
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		result = toRulePayload(rule, nodes)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.create", "FORWARDING_RULE", rule.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) GetRule(ctx context.Context, identity InternalIdentity, ruleID string) (RulePayload, error) {
	var result RulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canReadRule(identity, rule) {
			return ErrForbidden
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		result = toRulePayload(rule, nodes)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateRule(ctx context.Context, identity InternalIdentity, ruleID string, input RuleMutationInput) (RulePayload, error) {
	input.ForwardingType = defaultForwardingType(input.ForwardingType)
	var result RulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canManageRule(identity, rule) {
			return ErrForbidden
		}
		if !input.EnabledSet {
			input.Enabled = rule.Enabled
		}
		oldNodeGroupID := rule.Binding.NodeGroupID
		if err := service.prepareRuleMutation(ctx, repositories, identity, rule.ID, input, input.Enabled); err != nil {
			return err
		}
		now := service.timestamp()
		rule.Name = input.Name
		rule.Tags = input.Tags
		rule.Enabled = input.Enabled
		rule.Status = "DISABLED"
		if input.Enabled {
			rule.Status = "ENABLED"
		}
		rule.ForwardingType = input.ForwardingType
		rule.Protocol = input.Protocol
		rule.MatchType = input.Match.Type
		rule.SNIHostname = input.Match.SNIHostname
		rule.TargetType = input.Upstream.Type
		rule.TargetID = input.Upstream.TargetID
		rule.TargetGroupID = input.Upstream.TargetGroupID
		rule.ProxyProtocolIn = defaultProxyProtocol(input.ProxyProtocol.In)
		rule.ProxyProtocolOut = defaultProxyProtocol(input.ProxyProtocol.Out)
		rule.ConfigVersion++
		rule.UpdatedAt = now
		binding := inboundBindingForRule(identity.OrganizationID, input, now)
		if err := repositories.Rules().UpdateRule(ctx, rule, binding, input.Tags, now, service.newID); err != nil {
			return err
		}
		if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, oldNodeGroupID, now); err != nil {
			return err
		}
		if binding.NodeGroupID != oldNodeGroupID {
			if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, binding.NodeGroupID, now); err != nil {
				return err
			}
		}
		rule, err = repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, rule.ID)
		if err != nil {
			return err
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		result = toRulePayload(rule, nodes)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.update", "FORWARDING_RULE", rule.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteRule(ctx context.Context, identity InternalIdentity, ruleID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canManageRule(identity, rule) {
			return ErrForbidden
		}
		now := service.timestamp()
		if err := repositories.Rules().DeleteRule(ctx, identity.OrganizationID, ruleID, now); err != nil {
			return err
		}
		if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID, now); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.delete", "FORWARDING_RULE", ruleID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) EnableRule(ctx context.Context, identity InternalIdentity, ruleID string) (RulePayload, error) {
	return service.setRuleEnabled(ctx, identity, ruleID, true)
}

func (service *ControlService) DisableRule(ctx context.Context, identity InternalIdentity, ruleID string) (RulePayload, error) {
	return service.setRuleEnabled(ctx, identity, ruleID, false)
}

func (service *ControlService) CopyRule(ctx context.Context, identity InternalIdentity, ruleID string, input RuleCopyInput) (RulePayload, error) {
	var result RulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		source, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canManageRule(identity, source) {
			return ErrForbidden
		}
		if err := service.prepareRuleMutation(ctx, repositories, identity, source.ID, inputFromRule(source), false); err != nil {
			return err
		}
		if err := ensureRuleLimitAvailable(ctx, repositories, identity, source.OwnerUserID, 1); err != nil {
			return err
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = defaultRuleCopyName(source.Name)
		}
		tags := input.Tags
		if !input.TagsSet {
			tags = source.Tags
		}
		now := service.timestamp()
		copied := source
		copied.ID = service.newID()
		copied.Name = name
		copied.Enabled = false
		copied.Status = "DISABLED"
		copied.ForwardingType = defaultForwardingType(source.ForwardingType)
		copied.CreatedAt = now
		copied.UpdatedAt = now
		copied.ConfigVersion = 0
		if err := repositories.Rules().CreateRule(ctx, copied, source.Binding, tags, now, service.newID); err != nil {
			return err
		}
		copied, err = repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, copied.ID)
		if err != nil {
			return err
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, copied.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		result = toRulePayload(copied, nodes)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.copy", "FORWARDING_RULE", copied.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RuleTraffic(ctx context.Context, identity InternalIdentity, ruleID string) (RuleTrafficPayload, error) {
	var result RuleTrafficPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		canReadAllTraffic := service.hasPermission(identity, string(domain.PermissionTrafficReadAll))
		canReadOwnTraffic := service.hasPermission(identity, string(domain.PermissionTrafficReadOwn)) && rule.OwnerUserID == identity.UserID
		if !canReadAllTraffic && !canReadOwnTraffic {
			return ErrForbidden
		}
		if !service.canUseNodeGroup(identity, rule.Binding.NodeGroupID) {
			return ErrForbidden
		}
		traffic, err := repositories.Rules().SumRuleTraffic(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		result = RuleTrafficPayload{UploadBytes: traffic.UploadBytes, DownloadBytes: traffic.DownloadBytes, TCPConnections: traffic.TCPConnections, UDPPackets: traffic.UDPPackets, LimitMode: "TOTAL"}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) ExportRules(ctx context.Context, identity InternalIdentity, ruleIDs []string) (RulesExportPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionRulesReadOwn)) &&
		!service.hasPermission(identity, string(domain.PermissionRulesReadAll)) &&
		!service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) &&
		!service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return RulesExportPayload{}, ErrForbidden
	}
	selectedRuleIDs := map[string]bool{}
	for _, ruleID := range ruleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID != "" {
			selectedRuleIDs[ruleID] = true
		}
	}
	var result RulesExportPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rules, err := repositories.Rules().ListRulesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = RulesExportPayload{
			SchemaVersion: "rules.export.v1",
			ExportedAt:    service.timestamp(),
			Rules:         make([]PortableRulePayload, 0, len(rules)),
			Targets:       make([]PortableTargetPayload, 0),
			TargetGroups:  make([]PortableTargetGroupPayload, 0),
		}
		readableRules := make([]repo.RuleRecord, 0, len(rules))
		referencedTargets := map[string]bool{}
		referencedTargetGroups := map[string]bool{}
		for _, rule := range rules {
			if !service.canReadRule(identity, rule) {
				continue
			}
			if len(selectedRuleIDs) > 0 && !selectedRuleIDs[rule.ID] {
				continue
			}
			if rule.TargetType == "TARGET" {
				referencedTargets[rule.TargetID] = true
			}
			if rule.TargetType == "TARGET_GROUP" {
				referencedTargetGroups[rule.TargetGroupID] = true
			}
			readableRules = append(readableRules, rule)
		}
		targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		referencedGroupRecords := make([]repo.TargetGroupRecord, 0)
		for _, group := range targetGroups {
			if !referencedTargetGroups[group.ID] {
				continue
			}
			referencedGroupRecords = append(referencedGroupRecords, group)
			for _, member := range group.Members {
				referencedTargets[member.TargetID] = true
			}
		}
		sort.SliceStable(referencedGroupRecords, func(left int, right int) bool {
			if referencedGroupRecords[left].Name != referencedGroupRecords[right].Name {
				return referencedGroupRecords[left].Name < referencedGroupRecords[right].Name
			}
			return referencedGroupRecords[left].ID < referencedGroupRecords[right].ID
		})
		targetGroupRefsByID := make(map[string]string, len(referencedGroupRecords))
		for index, group := range referencedGroupRecords {
			targetGroupRefsByID[group.ID] = portableRef("target_group", index)
		}
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		referencedTargetRecords := make([]repo.TargetRecord, 0)
		for _, target := range targets {
			if referencedTargets[target.ID] {
				referencedTargetRecords = append(referencedTargetRecords, target)
			}
		}
		sort.SliceStable(referencedTargetRecords, func(left int, right int) bool {
			if referencedTargetRecords[left].Name != referencedTargetRecords[right].Name {
				return referencedTargetRecords[left].Name < referencedTargetRecords[right].Name
			}
			if referencedTargetRecords[left].Host != referencedTargetRecords[right].Host {
				return referencedTargetRecords[left].Host < referencedTargetRecords[right].Host
			}
			if referencedTargetRecords[left].Port != referencedTargetRecords[right].Port {
				return referencedTargetRecords[left].Port < referencedTargetRecords[right].Port
			}
			return referencedTargetRecords[left].ID < referencedTargetRecords[right].ID
		})
		targetRefsByID := make(map[string]string, len(referencedTargetRecords))
		for index, target := range referencedTargetRecords {
			targetRefsByID[target.ID] = portableRef("target", index)
			result.Targets = append(result.Targets, toPortableTargetPayload(target, targetRefsByID[target.ID]))
		}
		for _, group := range referencedGroupRecords {
			result.TargetGroups = append(result.TargetGroups, toPortableTargetGroupPayload(group, targetGroupRefsByID[group.ID], targetRefsByID))
		}
		for _, rule := range readableRules {
			result.Rules = append(result.Rules, toPortableRulePayload(rule, targetRefsByID, targetGroupRefsByID))
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.export", "FORWARDING_RULE", "", ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) ImportRules(ctx context.Context, identity InternalIdentity, input RuleImportInput) (RulesImportResult, error) {
	if !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return RulesImportResult{}, ErrForbidden
	}
	source, err := service.ruleImportSource(input)
	if err != nil {
		return RulesImportResult{}, err
	}
	if source.Payload.SchemaVersion != "rules.export.v1" {
		return RulesImportResult{}, validationFieldError("source_text.schema_version", "Import source_text must contain a rules.export.v1 payload.", map[string]any{
			"actual":   source.Payload.SchemaVersion,
			"expected": "rules.export.v1",
		})
	}
	input.Entry.NodeGroupID = strings.TrimSpace(input.Entry.NodeGroupID)
	input.Entry.ListenIP = strings.TrimSpace(input.Entry.ListenIP)
	if input.Entry.NodeGroupID == "" || input.Entry.ListenIP == "" {
		return RulesImportResult{}, validationError("Import entry requires node_group_id and listen_ip.", map[string]any{
			"entry.node_group_id_present": input.Entry.NodeGroupID != "",
			"entry.listen_ip_present":     input.Entry.ListenIP != "",
		})
	}
	result := RulesImportResult{DryRun: input.DryRun, Skipped: source.Skipped, Errors: append([]RuleImportIssue{}, source.Errors...), Warnings: []RuleImportIssue{}}
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		affectedNodeGroups := map[string]bool{}
		now := service.timestamp()
		if !service.canUseNodeGroup(identity, input.Entry.NodeGroupID) {
			return ErrForbidden
		}
		if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, input.Entry.NodeGroupID); err != nil {
			return err
		}
		targetIDsByRef, err := service.resolveImportTargets(ctx, repositories, identity, source.Payload.Targets, now, &result)
		if err != nil {
			return err
		}
		targetGroupIDsByRef, err := service.resolveImportTargetGroups(ctx, repositories, identity, source.Payload.TargetGroups, targetIDsByRef, now, &result)
		if err != nil {
			return err
		}
		simulatedEnabled, err := repositories.Rules().ListEnabledInboundBindings(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		additionalRules := 0
		for index, rulePayload := range source.Payload.Rules {
			ruleInput, err := ruleInputFromPortablePayload(rulePayload, input.Entry, targetIDsByRef, targetGroupIDsByRef)
			if err != nil {
				result.Errors = append(result.Errors, importIssueWithReason("IMPORT_RULE_INVALID", "rules", index, err, nil))
				result.Skipped++
				continue
			}
			quotaAdditional := 1
			if input.DryRun {
				quotaAdditional = additionalRules + 1
			}
			if err := ensureRuleLimitAvailable(ctx, repositories, identity, identity.UserID, quotaAdditional); err != nil {
				result.Errors = append(result.Errors, importIssueFromError("rules", index, err))
				result.Skipped++
				continue
			}
			enabled := true
			prepareErr := service.prepareRuleMutation(ctx, repositories, identity, "", ruleInput, true)
			if prepareErr == nil {
				nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, ruleInput.NodeGroupID)
				if err != nil {
					return err
				}
				if err := ensureNoRuleConflicts(ctx, repositories, identity.OrganizationID, simulatedEnabled, "", ruleInput, nodes); err != nil {
					prepareErr = err
				}
			}
			if prepareErr != nil {
				if err := service.prepareRuleImportDisabled(ctx, repositories, identity, ruleInput); err != nil {
					result.Errors = append(result.Errors, importIssueWithReason("IMPORT_RULE_INVALID", "rules", index, err, nil))
					result.Skipped++
					continue
				}
				enabled = false
				result.Warnings = append(result.Warnings, importIssueWithReason("IMPORT_RULE_DISABLED", "rules", index, prepareErr, nil))
			}
			additionalRules++
			if input.DryRun {
				if enabled {
					simulatedEnabled = append(simulatedEnabled, simulatedEnabledRule(identity, ruleInput, fmt.Sprintf("import_%d", index), now))
				}
				continue
			}
			status := "DISABLED"
			if enabled {
				status = "ENABLED"
			}
			ruleInput.Enabled = enabled
			binding := inboundBindingForRule(identity.OrganizationID, ruleInput, now)
			rule := repo.RuleRecord{
				ID:               service.newID(),
				OrganizationID:   identity.OrganizationID,
				OwnerUserID:      identity.UserID,
				Name:             ruleInput.Name,
				Enabled:          enabled,
				Status:           status,
				ForwardingType:   ruleInput.ForwardingType,
				Protocol:         ruleInput.Protocol,
				MatchType:        ruleInput.Match.Type,
				InboundBindingID: binding.ID,
				SNIHostname:      ruleInput.Match.SNIHostname,
				TargetType:       ruleInput.Upstream.Type,
				TargetID:         ruleInput.Upstream.TargetID,
				TargetGroupID:    ruleInput.Upstream.TargetGroupID,
				ProxyProtocolIn:  defaultProxyProtocol(ruleInput.ProxyProtocol.In),
				ProxyProtocolOut: defaultProxyProtocol(ruleInput.ProxyProtocol.Out),
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			if err := repositories.Rules().CreateRule(ctx, rule, binding, ruleInput.Tags, now, service.newID); err != nil {
				result.Errors = append(result.Errors, importIssueFromError("rules", index, err))
				result.Skipped++
				continue
			}
			if enabled {
				affectedNodeGroups[binding.NodeGroupID] = true
				simulatedEnabled = append(simulatedEnabled, simulatedEnabledRule(identity, ruleInput, rule.ID, now))
			}
			result.Created++
		}
		if input.DryRun {
			return errImportDryRunRollback
		}
		for nodeGroupID := range affectedNodeGroups {
			if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, nodeGroupID, now); err != nil {
				return err
			}
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.import", "FORWARDING_RULE", "", ""))
	})
	if errors.Is(err, errImportDryRunRollback) {
		return result, nil
	}
	return result, mapServiceError(err)
}

func (service *ControlService) BatchRules(ctx context.Context, identity InternalIdentity, input RuleBatchInput) (RuleBatchResult, error) {
	if !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return RuleBatchResult{}, ErrForbidden
	}
	action := strings.ToUpper(strings.TrimSpace(input.Action))
	switch action {
	case "ENABLE", "DISABLE", "DELETE":
	default:
		return RuleBatchResult{}, ErrInvalidInput
	}
	seen := map[string]bool{}
	ruleIDs := make([]string, 0, len(input.RuleIDs))
	for _, ruleID := range input.RuleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID == "" || seen[ruleID] {
			continue
		}
		seen[ruleID] = true
		ruleIDs = append(ruleIDs, ruleID)
	}
	if len(ruleIDs) == 0 {
		return RuleBatchResult{}, ErrInvalidInput
	}
	result := RuleBatchResult{Action: action, Total: len(ruleIDs), Results: make([]RuleBatchItemResult, 0, len(ruleIDs))}
	for _, ruleID := range ruleIDs {
		item := RuleBatchItemResult{RuleID: ruleID, Status: "SUCCEEDED"}
		var err error
		switch action {
		case "ENABLE":
			_, err = service.EnableRule(ctx, identity, ruleID)
		case "DISABLE":
			_, err = service.DisableRule(ctx, identity, ruleID)
		case "DELETE":
			err = service.DeleteRule(ctx, identity, ruleID)
		}
		if err != nil {
			payload := ErrorPayloadForError(err)
			item.Status = "FAILED"
			item.Error = &payload
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.Results = append(result.Results, item)
	}
	return result, nil
}

func (service *ControlService) resolveImportTargets(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, targets []PortableTargetPayload, now string, result *RulesImportResult) (map[string]string, error) {
	targetIDsByRef := map[string]string{}
	existingTargets, err := repositories.Targets().ListTargetsByOrganization(ctx, identity.OrganizationID)
	if err != nil {
		return nil, err
	}
	targetsByHostPort := map[string]repo.TargetRecord{}
	for _, target := range existingTargets {
		key := importTargetHostPortKey(target.Host, target.Port)
		if _, exists := targetsByHostPort[key]; !exists {
			targetsByHostPort[key] = target
		}
	}
	canCreateTargets := service.hasPermission(identity, string(domain.PermissionTargetsManage))
	for index, target := range targets {
		ref := strings.TrimSpace(target.Ref)
		name := strings.TrimSpace(target.Name)
		host := strings.TrimSpace(target.Host)
		if ref == "" || name == "" || len(name) > 120 || host == "" || len(host) > 253 || strings.ContainsAny(host, " \t\r\n") || target.Port < 1 || target.Port > 65535 {
			result.Errors = append(result.Errors, newRuleImportIssue("IMPORT_TARGET_INVALID", "targets", index, map[string]any{
				"ref_present":  ref != "",
				"name_present": name != "",
				"host_present": host != "",
				"host":         host,
				"port":         target.Port,
			}))
			continue
		}
		if _, exists := targetIDsByRef[ref]; exists {
			result.Errors = append(result.Errors, newRuleImportIssue("IMPORT_TARGET_DUPLICATE_REF", "targets", index, map[string]any{
				"ref": ref,
			}))
			continue
		}
		if existing, ok := targetsByHostPort[importTargetHostPortKey(host, target.Port)]; ok {
			targetIDsByRef[ref] = existing.ID
			continue
		}
		if !canCreateTargets {
			result.Errors = append(result.Errors, importIssueWithReason("IMPORT_TARGET_CREATE_FORBIDDEN", "targets", index, ErrForbidden, map[string]any{
				"ref":  ref,
				"host": host,
				"port": target.Port,
			}))
			continue
		}
		record := repo.TargetRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           name,
			Host:           host,
			Port:           target.Port,
			Enabled:        target.Enabled,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.Targets().CreateTarget(ctx, record); err != nil {
			result.Errors = append(result.Errors, importIssueFromError("targets", index, err))
			continue
		}
		targetIDsByRef[ref] = record.ID
		targetsByHostPort[importTargetHostPortKey(host, target.Port)] = record
	}
	return targetIDsByRef, nil
}

func (service *ControlService) resolveImportTargetGroups(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, groups []PortableTargetGroupPayload, targetIDsByRef map[string]string, now string, result *RulesImportResult) (map[string]string, error) {
	targetGroupIDsByRef := map[string]string{}
	existingGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, identity.OrganizationID)
	if err != nil {
		return nil, err
	}
	targetGroupsByName := map[string]repo.TargetGroupRecord{}
	for _, group := range existingGroups {
		if _, exists := targetGroupsByName[group.Name]; !exists {
			targetGroupsByName[group.Name] = group
		}
	}
	canCreateTargets := service.hasPermission(identity, string(domain.PermissionTargetsManage))
	for index, group := range groups {
		ref := strings.TrimSpace(group.Ref)
		name := strings.TrimSpace(group.Name)
		description := strings.TrimSpace(group.Description)
		scheduler := strings.ToUpper(strings.TrimSpace(group.Scheduler))
		if scheduler == "" {
			scheduler = targetGroupSchedulerPriorityIPHash
		}
		if ref == "" || name == "" || len(name) > 120 || len(description) > 1000 || scheduler != targetGroupSchedulerPriorityIPHash {
			result.Errors = append(result.Errors, newRuleImportIssue("IMPORT_TARGET_GROUP_INVALID", "target_groups", index, map[string]any{
				"ref_present":  ref != "",
				"name_present": name != "",
				"scheduler":    scheduler,
			}))
			continue
		}
		if _, exists := targetGroupIDsByRef[ref]; exists {
			result.Errors = append(result.Errors, newRuleImportIssue("IMPORT_TARGET_GROUP_DUPLICATE_REF", "target_groups", index, map[string]any{
				"ref": ref,
			}))
			continue
		}
		if existing, ok := targetGroupsByName[name]; ok {
			targetGroupIDsByRef[ref] = existing.ID
			continue
		}
		if !canCreateTargets {
			result.Errors = append(result.Errors, importIssueWithReason("IMPORT_TARGET_GROUP_CREATE_FORBIDDEN", "target_groups", index, ErrForbidden, map[string]any{
				"ref":  ref,
				"name": name,
			}))
			continue
		}
		members := make([]TargetGroupMemberInput, 0, len(group.Members))
		seenMembers := map[string]bool{}
		memberValid := true
		for memberIndex, member := range group.Members {
			targetRef := strings.TrimSpace(member.TargetRef)
			targetID, ok := targetIDsByRef[targetRef]
			if targetRef == "" || !ok || member.Priority < 0 || seenMembers[targetID] {
				result.Errors = append(result.Errors, newRuleImportIssue("IMPORT_TARGET_GROUP_MEMBER_INVALID", "target_groups", index, map[string]any{
					"member_index":        memberIndex,
					"target_ref":          targetRef,
					"target_ref_resolved": ok,
					"priority":            member.Priority,
				}))
				memberValid = false
				continue
			}
			seenMembers[targetID] = true
			members = append(members, TargetGroupMemberInput{TargetID: targetID, Priority: member.Priority, Enabled: member.Enabled})
		}
		if !memberValid {
			continue
		}
		record := repo.TargetGroupRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           name,
			Description:    description,
			Scheduler:      targetGroupSchedulerPriorityIPHash,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.TargetGroups().CreateTargetGroup(ctx, record, toTargetGroupMemberRecords(identity.OrganizationID, record.ID, members), now, service.newID); err != nil {
			result.Errors = append(result.Errors, importIssueFromError("target_groups", index, err))
			continue
		}
		targetGroupIDsByRef[ref] = record.ID
		targetGroupsByName[name] = record
	}
	return targetGroupIDsByRef, nil
}

func (service *ControlService) prepareRuleImportDisabled(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, input RuleMutationInput) error {
	if !service.canUseNodeGroup(identity, input.NodeGroupID) {
		return ErrForbidden
	}
	if err := validateRuleForwardingType(input.ForwardingType); err != nil {
		return err
	}
	if err := validateRuleMatchType(input.Match.Type); err != nil {
		return err
	}
	if err := validateRuleBindingShape(input, ""); err != nil {
		return err
	}
	if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, input.NodeGroupID); err != nil {
		return err
	}
	nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, input.NodeGroupID)
	if err != nil {
		return err
	}
	if !nodesShareListenIP(nodes, input.ListenIP) {
		return validationError("Selected import entry listen_ip is not available on every node in the node group.", map[string]any{
			"node_group_id": input.NodeGroupID,
			"listen_ip":     input.ListenIP,
		})
	}
	return ensureUpstreamExists(ctx, repositories, identity.OrganizationID, input.Upstream)
}

func importTargetHostPortKey(host string, port int) string {
	return strings.ToLower(strings.TrimSpace(host)) + ":" + fmt.Sprint(port)
}

func (service *ControlService) ListRules(ctx context.Context, identity InternalIdentity) ([]RulePayload, error) {
	if !service.hasRuleReadOrManagePermission(identity) {
		return nil, ErrForbidden
	}
	result := make([]RulePayload, 0)
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rules, err := repositories.Rules().ListRulesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if !service.canReadRule(identity, rule) {
				continue
			}
			nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
			if err != nil {
				return err
			}
			result = append(result, toRulePayload(rule, nodes))
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) setRuleEnabled(ctx context.Context, identity InternalIdentity, ruleID string, enabled bool) (RulePayload, error) {
	var result RulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canManageRule(identity, rule) {
			return ErrForbidden
		}
		if enabled {
			input := inputFromRule(rule)
			if err := service.prepareRuleMutation(ctx, repositories, identity, rule.ID, input, true); err != nil {
				return err
			}
		}
		rule.Enabled = enabled
		if enabled {
			rule.Status = "ENABLED"
		} else {
			rule.Status = "DISABLED"
		}
		rule.UpdatedAt = service.timestamp()
		rule.ConfigVersion++
		if err := repositories.Rules().UpdateRule(ctx, rule, rule.Binding, rule.Tags, rule.UpdatedAt, service.newID); err != nil {
			return err
		}
		if err := bumpDesiredConfigForNodeGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID, rule.UpdatedAt); err != nil {
			return err
		}
		rule, err = repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, rule.ID)
		if err != nil {
			return err
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		result = toRulePayload(rule, nodes)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "rules.status", "FORWARDING_RULE", rule.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) prepareRuleMutation(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, ruleID string, input RuleMutationInput, occupiesPort bool) error {
	if !service.canUseNodeGroup(identity, input.NodeGroupID) {
		return ErrForbidden
	}
	if err := validateRuleForwardingType(input.ForwardingType); err != nil {
		return err
	}
	if err := validateRuleMatchType(input.Match.Type); err != nil {
		return err
	}
	if err := validateRuleBindingShape(input, ruleID); err != nil {
		return err
	}
	if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, input.NodeGroupID); err != nil {
		return err
	}
	if err := ensureUpstreamAvailable(ctx, repositories, identity.OrganizationID, input.Upstream); err != nil {
		return err
	}
	nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, input.NodeGroupID)
	if err != nil {
		return err
	}
	if !nodesCoverListenIPAndPort(nodes, input.ListenIP, input.Protocol, input.Port) {
		return validationError("Selected entry does not cover the rule listen_ip, protocol, and port.", map[string]any{
			"node_group_id": input.NodeGroupID,
			"listen_ip":     input.ListenIP,
			"protocol":      input.Protocol,
			"port":          input.Port,
		})
	}
	if occupiesPort {
		existing, err := repositories.Rules().ListEnabledInboundBindings(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		if err := ensureNoRuleConflicts(ctx, repositories, identity.OrganizationID, existing, ruleID, input, nodes); err != nil {
			return err
		}
	}
	return nil
}

func ensureNoRuleConflicts(ctx context.Context, repositories repo.Repositories, organizationID string, existing []repo.RuleRecord, ruleID string, input RuleMutationInput, candidateNodes []repo.NodeRecord) error {
	candidates := bindingsForNodes(candidateNodes, input.NodeGroupID, input.ListenIP, input.Protocol, input.Port, input.Match.Type, input.Match.SNIHostname, defaultProxyProtocol(input.ProxyProtocol.In), ruleID)
	for _, existingRule := range existing {
		if existingRule.ID == ruleID {
			continue
		}
		existingNodes, err := nodesInGroup(ctx, repositories, organizationID, existingRule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		existingBindings := bindingsForNodes(existingNodes, existingRule.Binding.NodeGroupID, existingRule.Binding.ListenIP, existingRule.Protocol, existingRule.Binding.Port, existingRule.MatchType, existingRule.SNIHostname, existingRule.ProxyProtocolIn, existingRule.ID)
		for _, candidate := range candidates {
			if err := domain.ValidateInboundBindingConflict(existingBindings, candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindingsForNodes(nodes []repo.NodeRecord, nodeGroupID string, listenIP string, protocol string, port int, matchType string, sni string, proxyProtocolIn string, ruleID string) []domain.InboundBinding {
	bindings := make([]domain.InboundBinding, 0, len(nodes))
	for _, node := range nodes {
		bindings = append(bindings, domain.InboundBinding{NodeID: node.ID, ListenIP: listenIP, Protocol: domain.Protocol(protocol), Port: port, MatchType: domain.MatchType(matchType), SNI: sni, ProxyProtocolIn: proxyProtocolIn, ForwardRule: ruleID})
	}
	return bindings
}

func validateRuleBindingShape(input RuleMutationInput, ruleID string) error {
	bindings := bindingsForNodes([]repo.NodeRecord{{ID: "shape-check"}}, input.NodeGroupID, input.ListenIP, input.Protocol, input.Port, input.Match.Type, input.Match.SNIHostname, defaultProxyProtocol(input.ProxyProtocol.In), ruleID)
	if len(bindings) != 1 {
		return ErrInvalidInput
	}
	if err := domain.ValidateInboundBindingConflict(nil, bindings[0]); err != nil {
		return err
	}
	return nil
}

func ensureRuleLimitAvailable(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, ownerUserID string, additionalRules int) error {
	if additionalRules <= 0 {
		return nil
	}
	organization, err := repositories.Organizations().FindOrganizationByID(ctx, identity.OrganizationID)
	if err != nil {
		return err
	}
	quotas, err := repositories.Quotas().ListQuotasByOrganization(ctx, identity.OrganizationID)
	if err != nil {
		return err
	}
	var orgRuleCount int
	var orgRuleCountLoaded bool
	var ownerRuleCount int
	userRuleLimit := 0
	userRuleLimitSet := false
	userRuleUnlimited := false
	for _, quota := range quotas {
		if quota.Scope != "USER" || quota.SubjectUserID != ownerUserID {
			continue
		}
		if quota.RuleLimit == 0 && quota.TrafficLimitBytes == 0 {
			userRuleUnlimited = true
			continue
		}
		if quota.RuleLimit > 0 && (userRuleLimit == 0 || quota.RuleLimit < userRuleLimit) {
			userRuleLimit = quota.RuleLimit
			userRuleLimitSet = true
		}
	}
	if !userRuleLimitSet && userRuleUnlimited {
		userRuleLimitSet = true
	}
	if userRuleLimit > 0 {
		ownerRuleCount, err = repositories.Rules().CountRulesByOwner(ctx, identity.OrganizationID, ownerUserID)
		if err != nil {
			return err
		}
		if ownerRuleCount+additionalRules > userRuleLimit {
			return ErrQuotaExceeded
		}
	} else if !userRuleLimitSet && organization.DefaultRuleLimit > 0 {
		ownerRuleCount, err = repositories.Rules().CountRulesByOwner(ctx, identity.OrganizationID, ownerUserID)
		if err != nil {
			return err
		}
		if ownerRuleCount+additionalRules > organization.DefaultRuleLimit {
			return ErrQuotaExceeded
		}
	}
	for _, quota := range quotas {
		if quota.RuleLimit <= 0 {
			continue
		}
		switch quota.Scope {
		case "ORGANIZATION":
			if !orgRuleCountLoaded {
				orgRuleCount, err = repositories.Rules().CountRulesByOrganization(ctx, identity.OrganizationID)
				if err != nil {
					return err
				}
				orgRuleCountLoaded = true
			}
			if orgRuleCount+additionalRules > quota.RuleLimit {
				return ErrQuotaExceeded
			}
		}
	}
	return nil
}
