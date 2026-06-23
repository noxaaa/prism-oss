package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type healthDNSTestRuleRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestRuleRepository) ListRulesByOrganization(_ context.Context, organizationID string) ([]repo.RuleRecord, error) {
	result := make([]repo.RuleRecord, 0, len(repository.store.forwardingRules))
	for _, rule := range repository.store.forwardingRules {
		if rule.OrganizationID == organizationID && rule.DeletedAt == "" {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (repository healthDNSTestRuleRepository) FindRuleByID(_ context.Context, organizationID string, ruleID string) (repo.RuleRecord, error) {
	for _, rule := range repository.store.forwardingRules {
		if rule.OrganizationID == organizationID && rule.ID == ruleID && rule.DeletedAt == "" {
			return rule, nil
		}
	}
	return repo.RuleRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestRuleRepository) CreateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) UpdateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) DeleteRule(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) ListEnabledInboundBindings(_ context.Context, organizationID string) ([]repo.RuleRecord, error) {
	result := make([]repo.RuleRecord, 0, len(repository.store.forwardingRules))
	for _, rule := range repository.store.forwardingRules {
		if rule.OrganizationID == organizationID && rule.DeletedAt == "" && rule.Enabled {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (repository healthDNSTestRuleRepository) CountRulesByOrganization(context.Context, string) (int, error) {
	return 0, nil
}

func (repository healthDNSTestRuleRepository) CountRulesByOwner(context.Context, string, string) (int, error) {
	return 0, nil
}

func (repository healthDNSTestRuleRepository) SumRuleTraffic(context.Context, string, string) (repo.RuleTrafficRecord, error) {
	return repo.RuleTrafficRecord{}, nil
}

func (repository healthDNSTestRuleRepository) RecordNodeRuleTrafficAssignments(context.Context, string, string, []string, string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) RecordRuleTrafficReport(context.Context, string, string, repo.RuleTrafficReportRecord, []repo.RuleTrafficDeltaRecord, string, func() string) (bool, error) {
	return true, nil
}

func (repository healthDNSTestRuleRepository) ListRuleDeploymentsByOrganization(context.Context, string) ([]repo.RuleDeploymentRecord, error) {
	return nil, nil
}

func (repository healthDNSTestRuleRepository) ReplaceRuleDeploymentPending(context.Context, string, repo.RuleRecord, []repo.RuleDeploymentPendingRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) UpsertRuleDeploymentPending(context.Context, string, repo.RuleRecord, repo.RuleDeploymentPendingRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) RecordRuleDeploymentApplied(context.Context, string, string, int, []repo.RuleDeploymentAppliedRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) RecordRuleDeploymentFailures(context.Context, string, string, int, []repo.RuleDeploymentFailureRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) DeleteRuleDeploymentForNode(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestRuleRepository) DeleteRuleDeployments(context.Context, string, string) error {
	return nil
}
