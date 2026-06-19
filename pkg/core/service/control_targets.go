package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

const defaultTargetGroupMemberPriority = 10
const targetGroupSchedulerPriorityIPHash = "PRIORITY_IPHASH"

func (service *ControlService) ListTargets(ctx context.Context, identity InternalIdentity) ([]TargetPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsRead)) && !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return nil, ErrForbidden
	}
	var result []TargetPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toTargetPayloads(targets)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) TargetOptions(ctx context.Context, identity InternalIdentity, protocol string) ([]ResourceOption, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsRead)) && !service.hasPermission(identity, string(domain.PermissionTargetsManage)) && !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return nil, ErrForbidden
	}
	var result []ResourceOption
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = make([]ResourceOption, 0, len(targets))
		for _, target := range targets {
			option := ResourceOption{Value: target.ID, Label: fmt.Sprintf("%s (%s:%d)", target.Name, target.Host, target.Port)}
			if !target.Enabled {
				option.Disabled = true
				option.DisabledReason = "Target is disabled"
			}
			result = append(result, option)
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateTarget(ctx context.Context, identity InternalIdentity, input TargetMutationInput) (TargetPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return TargetPayload{}, ErrForbidden
	}
	var result TargetPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		target := repo.TargetRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           input.Name,
			Host:           input.Host,
			Port:           input.Port,
			Enabled:        input.Enabled,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.Targets().CreateTarget(ctx, target); err != nil {
			return err
		}
		if input.TargetGroupIDsProvided {
			if err := syncTargetGroupMemberships(ctx, repositories, identity.OrganizationID, target, input.TargetGroupIDs, now, service.newID); err != nil {
				return err
			}
		}
		result = toTargetPayload(target)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "targets.create", "TARGET", target.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateTarget(ctx context.Context, identity InternalIdentity, targetID string, input TargetMutationInput) (TargetPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return TargetPayload{}, ErrForbidden
	}
	var result TargetPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		target, err := repositories.Targets().FindTargetByID(ctx, identity.OrganizationID, targetID)
		if err != nil {
			return err
		}
		if !input.Enabled && target.Enabled {
			if err := ensureTargetNotUsedByRules(ctx, repositories, identity.OrganizationID, target.ID); err != nil {
				return err
			}
		}
		runtimeChanged := target.Host != input.Host || target.Port != input.Port || target.Enabled != input.Enabled
		target.Name = input.Name
		target.Host = input.Host
		target.Port = input.Port
		target.Enabled = input.Enabled
		target.UpdatedAt = service.timestamp()
		if err := repositories.Targets().UpdateTarget(ctx, target); err != nil {
			return err
		}
		if runtimeChanged {
			if err := bumpDesiredConfigForRulesUsingTarget(ctx, repositories, identity.OrganizationID, target.ID, target.UpdatedAt); err != nil {
				return err
			}
		}
		if input.TargetGroupIDsProvided {
			if err := syncTargetGroupMemberships(ctx, repositories, identity.OrganizationID, target, input.TargetGroupIDs, target.UpdatedAt, service.newID); err != nil {
				return err
			}
		}
		result = toTargetPayload(target)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "targets.update", "TARGET", target.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteTarget(ctx context.Context, identity InternalIdentity, targetID string) error {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.Targets().FindTargetByID(ctx, identity.OrganizationID, targetID); err != nil {
			return err
		}
		if err := ensureTargetNotUsedByRules(ctx, repositories, identity.OrganizationID, targetID); err != nil {
			return err
		}
		if err := repositories.Targets().DeleteTarget(ctx, identity.OrganizationID, targetID, service.timestamp()); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "targets.delete", "TARGET", targetID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) ListTargetGroups(ctx context.Context, identity InternalIdentity) ([]TargetGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsRead)) && !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return nil, ErrForbidden
	}
	var result []TargetGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		groups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toTargetGroupPayloads(groups)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) TargetGroupOptions(ctx context.Context, identity InternalIdentity, protocol string) ([]ResourceOption, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsRead)) && !service.hasPermission(identity, string(domain.PermissionTargetsManage)) && !service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) && !service.hasPermission(identity, string(domain.PermissionRulesManageAll)) {
		return nil, ErrForbidden
	}
	var result []ResourceOption
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		groups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		targetsByID := make(map[string]repo.TargetRecord, len(targets))
		for _, target := range targets {
			targetsByID[target.ID] = target
		}
		result = make([]ResourceOption, 0, len(groups))
		for _, group := range groups {
			option := ResourceOption{Value: group.ID, Label: fmt.Sprintf("%s (%d targets)", group.Name, len(group.Members))}
			if reason := targetGroupDisabledReason(group, targetsByID); reason != "" {
				option.Disabled = true
				option.DisabledReason = reason
			}
			result = append(result, option)
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateTargetGroup(ctx context.Context, identity InternalIdentity, input TargetGroupMutationInput) (TargetGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return TargetGroupPayload{}, ErrForbidden
	}
	var result TargetGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureTargetGroupMembersValid(ctx, repositories, identity.OrganizationID, input.Members); err != nil {
			return err
		}
		now := service.timestamp()
		group := repo.TargetGroupRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           input.Name,
			Description:    input.Description,
			Scheduler:      targetGroupSchedulerPriorityIPHash,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.TargetGroups().CreateTargetGroup(ctx, group, toTargetGroupMemberRecords(identity.OrganizationID, group.ID, input.Members), now, service.newID); err != nil {
			return err
		}
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, identity.OrganizationID, group.ID)
		if err != nil {
			return err
		}
		result = toTargetGroupPayload(group)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "target_groups.create", "TARGET_GROUP", group.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateTargetGroup(ctx context.Context, identity InternalIdentity, targetGroupID string, input TargetGroupMutationInput) (TargetGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return TargetGroupPayload{}, ErrForbidden
	}
	var result TargetGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, identity.OrganizationID, targetGroupID)
		if err != nil {
			return err
		}
		if !targetGroupSchedulerSupportedByCore(group) {
			return ErrInvalidInput
		}
		usedByRules, err := targetGroupUsedByRules(ctx, repositories, identity.OrganizationID, group.ID)
		if err != nil {
			return err
		}
		if err := ensureTargetGroupMembersValid(ctx, repositories, identity.OrganizationID, input.Members); err != nil {
			return err
		}
		if usedByRules {
			if err := ensureTargetGroupMembersUsable(ctx, repositories, identity.OrganizationID, input.Members); err != nil {
				return err
			}
		}
		runtimeChanged := targetGroupRuntimeChanged(group, input)
		group.Name = input.Name
		group.Description = input.Description
		group.Scheduler = targetGroupSchedulerPriorityIPHash
		group.UpdatedAt = service.timestamp()
		if err := repositories.TargetGroups().UpdateTargetGroup(ctx, group, toTargetGroupMemberRecords(identity.OrganizationID, group.ID, input.Members), group.UpdatedAt, service.newID); err != nil {
			return err
		}
		if usedByRules && runtimeChanged {
			if err := bumpDesiredConfigForRulesUsingTargetGroup(ctx, repositories, identity.OrganizationID, group.ID, group.UpdatedAt); err != nil {
				return err
			}
		}
		group, err = repositories.TargetGroups().FindTargetGroupByID(ctx, identity.OrganizationID, group.ID)
		if err != nil {
			return err
		}
		result = toTargetGroupPayload(group)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "target_groups.update", "TARGET_GROUP", group.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteTargetGroup(ctx context.Context, identity InternalIdentity, targetGroupID string) error {
	if !service.hasPermission(identity, string(domain.PermissionTargetsManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.TargetGroups().FindTargetGroupByID(ctx, identity.OrganizationID, targetGroupID); err != nil {
			return err
		}
		if err := ensureTargetGroupNotUsedByRules(ctx, repositories, identity.OrganizationID, targetGroupID); err != nil {
			return err
		}
		if err := repositories.TargetGroups().DeleteTargetGroup(ctx, identity.OrganizationID, targetGroupID, service.timestamp()); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "target_groups.delete", "TARGET_GROUP", targetGroupID, ""))
	})
	return mapServiceError(err)
}

func ensureTargetNotUsedByRules(ctx context.Context, repositories repo.Repositories, organizationID string, targetID string) error {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.TargetType == "TARGET" && rule.TargetID == targetID {
			return ErrConflict
		}
	}
	targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	targetGroupUsesTarget := map[string]bool{}
	for _, group := range targetGroups {
		for _, member := range group.Members {
			if member.TargetID == targetID {
				targetGroupUsesTarget[group.ID] = true
			}
		}
	}
	for _, rule := range rules {
		if rule.TargetType == "TARGET_GROUP" && targetGroupUsesTarget[rule.TargetGroupID] {
			return ErrConflict
		}
	}
	return nil
}

func ensureTargetGroupNotUsedByRules(ctx context.Context, repositories repo.Repositories, organizationID string, targetGroupID string) error {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.TargetType == "TARGET_GROUP" && rule.TargetGroupID == targetGroupID {
			return ErrConflict
		}
	}
	return nil
}

func ensureUpstreamAvailable(ctx context.Context, repositories repo.Repositories, organizationID string, upstream RuleUpstreamInput) error {
	switch upstream.Type {
	case "TARGET":
		target, err := repositories.Targets().FindTargetByID(ctx, organizationID, upstream.TargetID)
		if err != nil {
			return err
		}
		if !target.Enabled {
			return ErrInvalidInput
		}
	case "TARGET_GROUP":
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, upstream.TargetGroupID)
		if err != nil {
			return err
		}
		if !targetGroupSchedulerSupportedByCore(group) {
			return ErrInvalidInput
		}
		if len(group.Members) == 0 {
			return ErrInvalidInput
		}
		for _, member := range group.Members {
			if !member.Enabled {
				return ErrInvalidInput
			}
			target, err := repositories.Targets().FindTargetByID(ctx, organizationID, member.TargetID)
			if err != nil {
				return err
			}
			if !target.Enabled {
				return ErrInvalidInput
			}
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func ensureUpstreamExists(ctx context.Context, repositories repo.Repositories, organizationID string, upstream RuleUpstreamInput) error {
	switch upstream.Type {
	case "TARGET":
		_, err := repositories.Targets().FindTargetByID(ctx, organizationID, upstream.TargetID)
		return err
	case "TARGET_GROUP":
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, upstream.TargetGroupID)
		if err != nil {
			return err
		}
		if !targetGroupSchedulerSupportedByCore(group) {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func ensureTargetGroupMembersValid(ctx context.Context, repositories repo.Repositories, organizationID string, members []TargetGroupMemberInput) error {
	seen := map[string]struct{}{}
	for _, member := range members {
		if _, ok := seen[member.TargetID]; ok {
			return ErrInvalidInput
		}
		seen[member.TargetID] = struct{}{}
		if _, err := repositories.Targets().FindTargetByID(ctx, organizationID, member.TargetID); err != nil {
			return err
		}
	}
	return nil
}

func syncTargetGroupMemberships(ctx context.Context, repositories repo.Repositories, organizationID string, target repo.TargetRecord, targetGroupIDs []string, now string, nextID func() string) error {
	targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	targetGroupsByID := make(map[string]repo.TargetGroupRecord, len(targetGroups))
	for _, group := range targetGroups {
		targetGroupsByID[group.ID] = group
	}
	selected := make(map[string]struct{}, len(targetGroupIDs))
	for _, targetGroupID := range targetGroupIDs {
		if _, ok := targetGroupsByID[targetGroupID]; !ok {
			return repo.ErrNotFound
		}
		selected[targetGroupID] = struct{}{}
	}
	for _, group := range targetGroups {
		_, shouldContainTarget := selected[group.ID]
		members, changed := targetGroupMembersWithTarget(group.Members, target.ID, shouldContainTarget)
		if !changed {
			continue
		}
		if !targetGroupSchedulerSupportedByCore(group) {
			return ErrInvalidInput
		}
		input := TargetGroupMutationInput{Name: group.Name, Description: group.Description, Members: members}
		if err := ensureTargetGroupMembersValid(ctx, repositories, organizationID, input.Members); err != nil {
			return err
		}
		usedByRules, err := targetGroupUsedByRules(ctx, repositories, organizationID, group.ID)
		if err != nil {
			return err
		}
		if usedByRules {
			if err := ensureTargetGroupMembersUsable(ctx, repositories, organizationID, input.Members); err != nil {
				return err
			}
		}
		group.UpdatedAt = now
		if err := repositories.TargetGroups().UpdateTargetGroup(ctx, group, toTargetGroupMemberRecords(organizationID, group.ID, input.Members), now, nextID); err != nil {
			return err
		}
		if usedByRules && targetGroupRuntimeChanged(group, input) {
			if err := bumpDesiredConfigForRulesUsingTargetGroup(ctx, repositories, organizationID, group.ID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func targetGroupMembersWithTarget(members []repo.TargetGroupMemberRecord, targetID string, shouldContainTarget bool) ([]TargetGroupMemberInput, bool) {
	next := make([]TargetGroupMemberInput, 0, len(members)+1)
	found := false
	changed := false
	for _, member := range members {
		if member.TargetID == targetID {
			found = true
			if !shouldContainTarget {
				changed = true
				continue
			}
		}
		next = append(next, TargetGroupMemberInput{TargetID: member.TargetID, Priority: member.Priority, Enabled: member.Enabled})
	}
	if shouldContainTarget && !found {
		next = append(next, TargetGroupMemberInput{TargetID: targetID, Priority: defaultTargetGroupMemberPriority, Enabled: true})
		changed = true
	}
	return next, changed
}

func ensureTargetGroupMembersUsable(ctx context.Context, repositories repo.Repositories, organizationID string, members []TargetGroupMemberInput) error {
	if len(members) == 0 {
		return ErrInvalidInput
	}
	for _, member := range members {
		if !member.Enabled {
			return ErrInvalidInput
		}
		target, err := repositories.Targets().FindTargetByID(ctx, organizationID, member.TargetID)
		if err != nil {
			return err
		}
		if !target.Enabled {
			return ErrInvalidInput
		}
	}
	return nil
}

func targetGroupUsedByRules(ctx context.Context, repositories repo.Repositories, organizationID string, targetGroupID string) (bool, error) {
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return false, err
	}
	for _, rule := range rules {
		if rule.TargetType == "TARGET_GROUP" && rule.TargetGroupID == targetGroupID {
			return true, nil
		}
	}
	return false, nil
}

func targetGroupDisabledReason(group repo.TargetGroupRecord, targetsByID map[string]repo.TargetRecord) string {
	if !targetGroupSchedulerSupportedByCore(group) {
		return "Target group scheduler is not supported by this build"
	}
	if len(group.Members) == 0 {
		return "Target group has no targets"
	}
	for _, member := range group.Members {
		if !member.Enabled {
			return "Target group has disabled members"
		}
		target, ok := targetsByID[member.TargetID]
		if !ok {
			return "Target group contains an unavailable target"
		}
		if !target.Enabled {
			return "Target group contains a disabled target"
		}
	}
	return ""
}

func toTargetPayload(target repo.TargetRecord) TargetPayload {
	return TargetPayload{ID: target.ID, Name: target.Name, Host: target.Host, Port: target.Port, Enabled: target.Enabled}
}

func toTargetPayloads(targets []repo.TargetRecord) []TargetPayload {
	payloads := make([]TargetPayload, 0, len(targets))
	for _, target := range targets {
		payloads = append(payloads, toTargetPayload(target))
	}
	return payloads
}

func toTargetGroupMemberRecords(organizationID string, targetGroupID string, inputs []TargetGroupMemberInput) []repo.TargetGroupMemberRecord {
	members := make([]repo.TargetGroupMemberRecord, 0, len(inputs))
	for _, input := range inputs {
		members = append(members, repo.TargetGroupMemberRecord{OrganizationID: organizationID, TargetGroupID: targetGroupID, TargetID: input.TargetID, Priority: input.Priority, Enabled: input.Enabled})
	}
	return members
}

func targetGroupRuntimeChanged(group repo.TargetGroupRecord, input TargetGroupMutationInput) bool {
	if !targetGroupSchedulerSupportedByCore(group) {
		return true
	}
	if len(group.Members) != len(input.Members) {
		return true
	}
	existing := make([]TargetGroupMemberPayload, 0, len(group.Members))
	for _, member := range group.Members {
		existing = append(existing, TargetGroupMemberPayload{TargetID: member.TargetID, Priority: member.Priority, Enabled: member.Enabled})
	}
	next := make([]TargetGroupMemberPayload, 0, len(input.Members))
	for _, member := range input.Members {
		next = append(next, TargetGroupMemberPayload(member))
	}
	sort.Slice(existing, func(left int, right int) bool {
		if existing[left].TargetID != existing[right].TargetID {
			return existing[left].TargetID < existing[right].TargetID
		}
		if existing[left].Priority != existing[right].Priority {
			return existing[left].Priority < existing[right].Priority
		}
		return !existing[left].Enabled && existing[right].Enabled
	})
	sort.Slice(next, func(left int, right int) bool {
		if next[left].TargetID != next[right].TargetID {
			return next[left].TargetID < next[right].TargetID
		}
		if next[left].Priority != next[right].Priority {
			return next[left].Priority < next[right].Priority
		}
		return !next[left].Enabled && next[right].Enabled
	})
	for index := range existing {
		if existing[index] != next[index] {
			return true
		}
	}
	return false
}

func targetGroupSchedulerSupportedByCore(group repo.TargetGroupRecord) bool {
	return group.Scheduler == targetGroupSchedulerPriorityIPHash
}

func toTargetGroupPayload(group repo.TargetGroupRecord) TargetGroupPayload {
	return TargetGroupPayload{ID: group.ID, Name: group.Name, Description: group.Description, Scheduler: group.Scheduler, Members: toTargetGroupMemberPayloads(group.Members)}
}

func toTargetGroupPayloads(groups []repo.TargetGroupRecord) []TargetGroupPayload {
	payloads := make([]TargetGroupPayload, 0, len(groups))
	for _, group := range groups {
		payloads = append(payloads, toTargetGroupPayload(group))
	}
	return payloads
}

func toTargetGroupMemberPayloads(members []repo.TargetGroupMemberRecord) []TargetGroupMemberPayload {
	payloads := make([]TargetGroupMemberPayload, 0, len(members))
	for _, member := range members {
		payloads = append(payloads, TargetGroupMemberPayload{TargetID: member.TargetID, Priority: member.Priority, Enabled: member.Enabled})
	}
	return payloads
}
