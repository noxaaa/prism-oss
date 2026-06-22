package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

var errImportDryRunRollback = errors.New("rule import dry-run rollback")

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
		createdEnabledRules := []repo.RuleRecord{}
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
				FailurePolicy:    defaultFailurePolicy(ruleInput.FailurePolicy),
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
				rule.Binding = binding
				createdEnabledRules = append(createdEnabledRules, rule)
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
		for _, rule := range createdEnabledRules {
			if err := syncRuleDeploymentPending(ctx, repositories, identity.OrganizationID, rule, now, service.newID); err != nil {
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
