package service

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type AgentHelloInput struct {
	Version    string
	Commit     string
	BuildTime  string
	RemoteAddr string
}

func (service *ControlService) MarkNodeAgentConnected(ctx context.Context, organizationID string, nodeID string) error {
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.Nodes().MarkNodeAgentConnected(ctx, organizationID, nodeID, now); err != nil {
			return err
		}
		if strings.ToUpper(strings.TrimSpace(node.Status)) != "ONLINE" {
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, organizationID, node.GroupIDs, now)
			return err
		}
		return nil
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, organizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func (service *ControlService) MarkNodeAgentConnectedFromRemote(ctx context.Context, organizationID string, nodeID string, remoteAddr string) error {
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		if err := repositories.Nodes().MarkNodeAgentConnected(ctx, organizationID, nodeID, now); err != nil {
			return err
		}
		ip := publicRemoteIP(remoteAddr)
		dnsInputChanged := strings.ToUpper(strings.TrimSpace(node.Status)) != "ONLINE"
		if ip != nil {
			publisher, ok := repositories.Nodes().(interface {
				UpsertAutoNodeDNSPublishAddress(context.Context, string, string, string, string, string, func() string) error
			})
			if ok {
				addressType := "A"
				if ip.To4() == nil {
					addressType = "AAAA"
				}
				if !nodeHasEnabledDNSPublishAddress(node, addressType, ip.String(), "AUTO") || nodeHasOtherEnabledAutoDNSPublishAddress(node, addressType, ip.String()) {
					dnsInputChanged = true
				}
				if err := publisher.UpsertAutoNodeDNSPublishAddress(ctx, organizationID, nodeID, addressType, ip.String(), now, service.newID); err != nil {
					return err
				}
			}
		} else if nodeHasEnabledAutoDNSPublishAddress(node) {
			disabler, ok := repositories.Nodes().(interface {
				DisableAutoNodeDNSPublishAddresses(context.Context, string, string, string) error
			})
			if ok {
				dnsInputChanged = true
				if err := disabler.DisableAutoNodeDNSPublishAddresses(ctx, organizationID, nodeID, now); err != nil {
					return err
				}
			}
		}
		if dnsInputChanged {
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, organizationID, node.GroupIDs, now)
			return err
		}
		return nil
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, organizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func publicRemoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || isCarrierGradeNATIP(ip) {
		return nil
	}
	return ip
}

func isCarrierGradeNATIP(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 0x40
}

func (service *ControlService) RecordNodeAgentHello(ctx context.Context, organizationID string, nodeID string, input AgentHelloInput) (NodePayload, bool, error) {
	var result NodePayload
	shouldUpdate := false
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		nodeBefore, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		if err := repositories.Nodes().MarkNodeAgentConnected(ctx, organizationID, nodeID, now); err != nil {
			return err
		}
		dnsInputChanged := strings.ToUpper(strings.TrimSpace(nodeBefore.Status)) != "ONLINE"
		if ip := publicRemoteIP(input.RemoteAddr); ip != nil {
			publisher, ok := repositories.Nodes().(interface {
				UpsertAutoNodeDNSPublishAddress(context.Context, string, string, string, string, string, func() string) error
			})
			if ok {
				addressType := "A"
				if ip.To4() == nil {
					addressType = "AAAA"
				}
				if !nodeHasEnabledDNSPublishAddress(nodeBefore, addressType, ip.String(), "AUTO") || nodeHasOtherEnabledAutoDNSPublishAddress(nodeBefore, addressType, ip.String()) {
					dnsInputChanged = true
				}
				if err := publisher.UpsertAutoNodeDNSPublishAddress(ctx, organizationID, nodeID, addressType, ip.String(), now, service.newID); err != nil {
					return err
				}
			}
		} else if nodeHasEnabledAutoDNSPublishAddress(nodeBefore) {
			disabler, ok := repositories.Nodes().(interface {
				DisableAutoNodeDNSPublishAddresses(context.Context, string, string, string) error
			})
			if ok {
				dnsInputChanged = true
				if err := disabler.DisableAutoNodeDNSPublishAddresses(ctx, organizationID, nodeID, now); err != nil {
					return err
				}
			}
		}
		if dnsInputChanged {
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, organizationID, nodeBefore.GroupIDs, now)
			if err != nil {
				return err
			}
		}
		if err := repositories.Nodes().UpdateNodeAgentVersion(ctx, organizationID, nodeID, repo.NodeAgentVersionRecord{Version: input.Version, Commit: input.Commit, BuildTime: input.BuildTime}, now); err != nil {
			return err
		}
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		targetVersion := service.targetAgentVersion()
		if agentUpdateCompletedByHello(node, input.Version) {
			if err := repositories.Nodes().MarkNodeAgentUpdateSatisfied(ctx, organizationID, nodeID, node.DesiredAgentVersion, now); err != nil {
				return err
			}
			node.AgentUpdateStatus = "SUCCEEDED"
			node.AgentUpdateError = ""
			node.AgentUpdateStartedAt = defaultString(node.AgentUpdateStartedAt, now)
			node.AgentUpdateFinishedAt = now
		}
		if node.AgentAutoUpdateEnabled && shouldRequestAgentAutoUpdate(input.Version, targetVersion) {
			shouldUpdate = true
			if err := repositories.Nodes().MarkNodeAgentUpdateRequested(ctx, organizationID, nodeID, targetVersion, now); err != nil {
				return err
			}
			node.DesiredAgentVersion = targetVersion
			node.AgentUpdateStatus = "PENDING"
			node.AgentUpdateError = ""
			node.AgentUpdateStartedAt = now
			node.AgentUpdateFinishedAt = ""
		}
		if node.DesiredAgentVersion == "" {
			node.DesiredAgentVersion = targetVersion
		}
		result = service.toNodePayload(node)
		return nil
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, organizationID, affectedDNSRecordIDs)
	}
	return result, shouldUpdate, mapServiceError(err)
}

func (service *ControlService) UpdateNodeAgentUpdatePolicy(ctx context.Context, identity InternalIdentity, nodeID string, input AgentUpdatePolicyInput) (NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodePayload{}, ErrForbidden
	}
	var result NodePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, node.GroupIDs); err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.Nodes().UpdateNodeAgentUpdatePolicy(ctx, identity.OrganizationID, nodeID, input.Enabled, now); err != nil {
			return err
		}
		node.AgentAutoUpdateEnabled = input.Enabled
		node.UpdatedAt = now
		targetVersion := service.targetAgentVersion()
		if input.Enabled && shouldRequestAgentAutoUpdate(node.AgentVersion, targetVersion) {
			node, err = requestNodeAgentUpgrade(ctx, repositories.Nodes(), identity.OrganizationID, node, targetVersion, now)
			if err != nil {
				return err
			}
		} else {
			node, err = repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
			if err != nil {
				return err
			}
		}
		result = service.toNodePayload(node)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.agent_update_policy", "NODE", node.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RequestNodeAgentUpgrade(ctx context.Context, identity InternalIdentity, nodeID string) (NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodePayload{}, ErrForbidden
	}
	var result NodePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, node.GroupIDs); err != nil {
			return err
		}
		targetVersion := service.targetAgentVersion()
		if err := requireConcreteAgentUpdateTarget(targetVersion); err != nil {
			return err
		}
		now := service.timestamp()
		node, err = requestNodeAgentUpgrade(ctx, repositories.Nodes(), identity.OrganizationID, node, targetVersion, now)
		if err != nil {
			return err
		}
		result = service.toNodePayload(node)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.agent_upgrade", "NODE", node.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RequestNodeAgentUpgrades(ctx context.Context, identity InternalIdentity, input AgentUpgradeBatchInput) ([]NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return nil, ErrForbidden
	}
	var result []NodePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		selectedIDs := make(map[string]struct{}, len(input.NodeIDs))
		for _, nodeID := range input.NodeIDs {
			nodeID = strings.TrimSpace(nodeID)
			if nodeID != "" {
				selectedIDs[nodeID] = struct{}{}
			}
		}
		targetVersion := service.targetAgentVersion()
		if err := requireConcreteAgentUpdateTarget(targetVersion); err != nil {
			return err
		}
		now := service.timestamp()
		result = make([]NodePayload, 0)
		matchedIDs := make(map[string]struct{}, len(selectedIDs))
		for _, node := range nodes {
			if len(selectedIDs) > 0 {
				if _, ok := selectedIDs[node.ID]; !ok {
					continue
				}
				matchedIDs[node.ID] = struct{}{}
			}
			if err := service.ensureCanManageNodeGroups(identity, node.GroupIDs); err != nil {
				return err
			}
			updatedNode, err := requestNodeAgentUpgrade(ctx, repositories.Nodes(), identity.OrganizationID, node, targetVersion, now)
			if err != nil {
				return err
			}
			result = append(result, service.toNodePayload(updatedNode))
			if err := service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.agent_upgrade", "NODE", updatedNode.ID, "")); err != nil {
				return err
			}
		}
		if len(selectedIDs) > 0 && len(matchedIDs) != len(selectedIDs) {
			return repo.ErrNotFound
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RecordNodeAgentUpdateResult(ctx context.Context, organizationID string, nodeID string, status string, errorMessage string) error {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "SUCCEEDED" && status != "FAILED" && status != "RUNNING" {
		return ErrInvalidInput
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Nodes().RecordNodeAgentUpdateResult(ctx, organizationID, nodeID, status, errorMessage, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) PendingNodeAgentUpdate(ctx context.Context, organizationID string, nodeID string) (string, bool, error) {
	targetVersion := ""
	pending := false
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		if node.DesiredAgentVersion != "" && node.DesiredAgentVersion != node.AgentVersion && node.AgentUpdateStatus == "PENDING" {
			targetVersion = node.DesiredAgentVersion
			pending = true
		}
		return nil
	})
	return targetVersion, pending, mapServiceError(err)
}

func (service *ControlService) targetAgentVersion() string {
	configuredVersion := strings.TrimSpace(service.agentReleaseVersion)
	if configuredVersion == "" {
		return strings.TrimSpace(buildinfo.Version)
	}
	if strings.EqualFold(configuredVersion, "latest") && agentUpdateTargetIsConcrete(buildinfo.Version) {
		return strings.TrimSpace(buildinfo.Version)
	}
	return configuredVersion
}

func (service *ControlService) AgentReleaseVersion() string {
	return service.targetAgentVersion()
}

func shouldRequestAgentAutoUpdate(currentVersion string, targetVersion string) bool {
	currentVersion = strings.TrimSpace(currentVersion)
	targetVersion = strings.TrimSpace(targetVersion)
	return currentVersion != "" && agentUpdateTargetIsConcrete(targetVersion) && currentVersion != targetVersion
}

func requireConcreteAgentUpdateTarget(targetVersion string) error {
	if !agentUpdateTargetIsConcrete(targetVersion) {
		return ErrInvalidInput
	}
	return nil
}

func requestNodeAgentUpgrade(ctx context.Context, nodes repo.NodeRepository, organizationID string, node repo.NodeRecord, targetVersion string, now string) (repo.NodeRecord, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion != "" && strings.TrimSpace(node.AgentVersion) == targetVersion {
		if err := nodes.MarkNodeAgentUpdateSatisfied(ctx, organizationID, node.ID, targetVersion, now); err != nil {
			return repo.NodeRecord{}, err
		}
		node.DesiredAgentVersion = targetVersion
		node.AgentUpdateStatus = "SUCCEEDED"
		node.AgentUpdateError = ""
		node.AgentUpdateStartedAt = defaultString(node.AgentUpdateStartedAt, now)
		node.AgentUpdateFinishedAt = now
		return node, nil
	}
	if err := nodes.MarkNodeAgentUpdateRequested(ctx, organizationID, node.ID, targetVersion, now); err != nil {
		return repo.NodeRecord{}, err
	}
	node.DesiredAgentVersion = targetVersion
	node.AgentUpdateStatus = "PENDING"
	node.AgentUpdateError = ""
	node.AgentUpdateStartedAt = now
	node.AgentUpdateFinishedAt = ""
	return node, nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func agentUpdateCompletedByHello(node repo.NodeRecord, reportedVersion string) bool {
	reportedVersion = strings.TrimSpace(reportedVersion)
	desiredVersion := strings.TrimSpace(node.DesiredAgentVersion)
	return reportedVersion != "" && reportedVersion == desiredVersion && node.AgentUpdateStatus != "SUCCEEDED"
}

func agentUpdateTargetIsConcrete(version string) bool {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "", "latest", "dev", "unknown":
		return false
	default:
		return true
	}
}

func (service *ControlService) MarkNodeAgentDisconnected(ctx context.Context, organizationID string, nodeID string) error {
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.Nodes().MarkNodeAgentDisconnected(ctx, organizationID, nodeID, now); err != nil {
			return err
		}
		if strings.ToUpper(strings.TrimSpace(node.Status)) != "OFFLINE" {
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, organizationID, node.GroupIDs, now)
			return err
		}
		return nil
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, organizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func nodeHasEnabledDNSPublishAddress(node repo.NodeRecord, addressType string, address string, source string) bool {
	for _, candidate := range node.DNSPublishAddresses {
		if !candidate.Enabled || candidate.AddressType != addressType || candidate.Address != address {
			continue
		}
		if source != "" && candidate.Source != source {
			continue
		}
		return true
	}
	return false
}

func nodeHasEnabledAutoDNSPublishAddress(node repo.NodeRecord) bool {
	for _, candidate := range node.DNSPublishAddresses {
		if candidate.Enabled && candidate.Source == "AUTO" {
			return true
		}
	}
	return false
}

func nodeHasOtherEnabledAutoDNSPublishAddress(node repo.NodeRecord, addressType string, address string) bool {
	for _, candidate := range node.DNSPublishAddresses {
		if !candidate.Enabled || candidate.Source != "AUTO" || candidate.AddressType != addressType || candidate.Address == address {
			continue
		}
		return true
	}
	return false
}

func (service *ControlService) MarkMonitorAgentConnected(ctx context.Context, organizationID string, monitorID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().MarkMonitorAgentConnected(ctx, organizationID, monitorID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) MarkMonitorAgentDisconnected(ctx context.Context, organizationID string, monitorID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().MarkMonitorAgentDisconnected(ctx, organizationID, monitorID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) CompileNodeAgentConfig(ctx context.Context, organizationID string, nodeID string) (AgentConfig, error) {
	return service.CompileNodeAgentConfigForAgentProtocol(ctx, organizationID, nodeID, agent.CurrentProtocolVersion(), true)
}

func (service *ControlService) CompileNodeAgentConfigForAgentProtocol(ctx context.Context, organizationID string, nodeID string, protocolVersion agent.ProtocolVersion, protocolKnown bool) (AgentConfig, error) {
	var result AgentConfig
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		candidateRules := executableRulesForNode(node, rules)
		candidateConfigVersion := maxRuleRecordConfigVersion(candidateRules)
		ruleConfigs, err := toRuleConfigs(candidateRules, targets, targetGroups)
		if err != nil {
			return err
		}
		compiled, err := BasicAgentConfigCompiler{}.Compile(ctx, AgentConfigInput{
			NodeID:                  node.ID,
			NodeGroups:              node.GroupIDs,
			AgentProtocolVersion:    protocolVersion,
			AgentProtocolKnown:      protocolKnown,
			DataplaneMode:           node.DataplaneMode,
			DataplaneInstanceID:     node.DataplaneInstanceID,
			DataplaneConflictPolicy: node.DataplaneConflictPolicy,
			Rules:                   ruleConfigs,
		})
		if err != nil {
			return err
		}
		targetDesiredConfigVersion := node.DesiredConfigVersion
		if compiled.ConfigVersion > targetDesiredConfigVersion {
			targetDesiredConfigVersion = compiled.ConfigVersion
		}
		if candidateConfigVersion > targetDesiredConfigVersion {
			targetDesiredConfigVersion = candidateConfigVersion
		}
		if targetDesiredConfigVersion > node.DesiredConfigVersion {
			if err := repositories.Nodes().EnsureDesiredConfigVersionAtLeast(ctx, organizationID, nodeID, targetDesiredConfigVersion, service.timestamp()); err != nil {
				return err
			}
		}
		if targetDesiredConfigVersion > compiled.ConfigVersion {
			compiled.ConfigVersion = targetDesiredConfigVersion
			compiled.ConfigHash = configHash(compiled)
		}
		ruleIDs := make([]string, 0, len(compiled.Rules))
		for _, rule := range compiled.Rules {
			ruleIDs = append(ruleIDs, rule.ID)
		}
		if err := repositories.Rules().RecordNodeRuleTrafficAssignments(ctx, organizationID, nodeID, ruleIDs, service.timestamp()); err != nil {
			return err
		}
		result = compiled
		return nil
	})
	return result, mapServiceError(err)
}

func executableRulesForNode(node repo.NodeRecord, rules []repo.RuleRecord) []repo.RuleRecord {
	nodeGroups := make(map[string]struct{}, len(node.GroupIDs))
	for _, nodeGroupID := range node.GroupIDs {
		nodeGroups[nodeGroupID] = struct{}{}
	}
	filtered := make([]repo.RuleRecord, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if _, ok := nodeGroups[rule.Binding.NodeGroupID]; !ok {
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered
}

func maxRuleRecordConfigVersion(rules []repo.RuleRecord) int {
	maxVersion := 0
	for _, rule := range rules {
		if rule.ConfigVersion > maxVersion {
			maxVersion = rule.ConfigVersion
		}
	}
	return maxVersion
}

func (service *ControlService) NodeAgentConfigBehind(ctx context.Context, organizationID string, nodeID string, appliedConfigVersion int) (bool, error) {
	var result bool
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		if nodeConfigBackoffActive(node, service.now()) {
			result = false
			return nil
		}
		result = node.DesiredConfigVersion > appliedConfigVersion || node.ConfigStatus == "FAILED"
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) AcknowledgeNodeAgentConfig(ctx context.Context, organizationID string, nodeID string, configVersion int, configHash string, status string, errorMessage string, applyErrors []ConfigApplyErrorInput, resolvedDataplanes map[string]string) error {
	return service.AcknowledgeNodeAgentConfigForAgentProtocol(ctx, organizationID, nodeID, configVersion, configHash, agent.CurrentProtocolVersion(), true, status, errorMessage, applyErrors, resolvedDataplanes)
}

func (service *ControlService) AcknowledgeNodeAgentConfigForAgentProtocol(ctx context.Context, organizationID string, nodeID string, configVersion int, configHash string, protocolVersion agent.ProtocolVersion, protocolKnown bool, status string, errorMessage string, applyErrors []ConfigApplyErrorInput, resolvedDataplanes map[string]string) error {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPLIED" && status != "FAILED" {
		return ErrInvalidInput
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if len(errorMessage) > 1000 {
		errorMessage = errorMessage[:1000]
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		nowTime := service.now()
		now := nowTime.UTC().Format(time.RFC3339Nano)
		ack := repo.NodeConfigAckRecord{
			ConfigVersion:     configVersion,
			Status:            status,
			ErrorMessage:      errorMessage,
			DataplaneStatus:   NodeDataplaneStatusHealthy,
			DataplaneLastHash: strings.TrimSpace(configHash),
		}
		if status == "FAILED" {
			ack = nodeConfigFailedAck(node, configVersion, errorMessage, nowTime)
			ack.DataplaneStatus = NodeDataplaneStatusFailed
			ack.DataplaneError = errorMessage
		}
		var applied []repo.RuleDeploymentAppliedRecord
		partialProtocolApply := false
		if status == "APPLIED" {
			rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
			if err != nil {
				return err
			}
			var complete bool
			applied, complete, err = ruleDeploymentAppliedRecordsForNode(ctx, repositories, organizationID, node, rules, protocolVersion, protocolKnown, resolvedDataplanes)
			if err != nil {
				return err
			}
			if !complete {
				partialProtocolApply = true
				errorMessage = "agent protocol does not support all desired dataplane rules"
				ack = nodeConfigFailedAck(node, configVersion, errorMessage, nowTime)
				ack.DataplaneStatus = NodeDataplaneStatusFailed
				ack.DataplaneError = errorMessage
			}
		}
		if err := repositories.Nodes().RecordNodeConfigAck(ctx, organizationID, nodeID, ack, now); err != nil {
			return err
		}
		if configVersion != node.DesiredConfigVersion {
			return nil
		}
		if status == "APPLIED" {
			if len(applied) == 0 && partialProtocolApply {
				return nil
			}
			return repositories.Rules().RecordRuleDeploymentApplied(ctx, organizationID, nodeID, configVersion, applied, now, service.newID)
		}
		rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		rulesByID := make(map[string]repo.RuleRecord, len(rules))
		for _, rule := range rules {
			rulesByID[rule.ID] = rule
		}
		failures := ruleDeploymentFailuresFromApplyErrors(rulesByID, applyErrors)
		if len(failures) > 0 {
			if err := repositories.Rules().RecordRuleDeploymentFailures(ctx, organizationID, nodeID, configVersion, failures, now, service.newID); err != nil {
				return err
			}
		}
		failedRuleIDs := make([]string, 0, len(failures))
		for _, failure := range failures {
			failedRuleIDs = append(failedRuleIDs, failure.RuleID)
		}
		return service.applyRuleFailurePolicies(ctx, repositories, organizationID, failedRuleIDs, now)
	})
	return mapServiceError(err)
}

func toRuleConfigs(rules []repo.RuleRecord, targets []repo.TargetRecord, targetGroups []repo.TargetGroupRecord) ([]RuleConfig, error) {
	targetsByID := make(map[string]repo.TargetRecord, len(targets))
	for _, target := range targets {
		targetsByID[target.ID] = target
	}
	targetGroupsByID := make(map[string]repo.TargetGroupRecord, len(targetGroups))
	for _, group := range targetGroups {
		targetGroupsByID[group.ID] = group
	}
	configs := make([]RuleConfig, 0, len(rules))
	for _, rule := range rules {
		if !openCoreRuleRecordSupported(rule) {
			continue
		}
		config := RuleConfig{
			ID:               rule.ID,
			ConfigVersion:    rule.ConfigVersion,
			Enabled:          rule.Enabled,
			ForwardingType:   domain.ForwardingType(defaultForwardingType(rule.ForwardingType)),
			Protocol:         domain.Protocol(rule.Protocol),
			NodeGroupIDs:     []string{rule.Binding.NodeGroupID},
			ListenIP:         rule.Binding.ListenIP,
			Port:             rule.Binding.Port,
			PortSegments:     toAgentPortSegments(rulePortSegments(rule.Binding)),
			SendIP:           rule.SendIP,
			MatchType:        rule.MatchType,
			SNIHostname:      rule.SNIHostname,
			ProxyProtocolIn:  rule.ProxyProtocolIn,
			ProxyProtocolOut: rule.ProxyProtocolOut,
			Dataplane:        defaultDataplanePreference(rule.DataplanePreference),
		}
		switch rule.TargetType {
		case "TARGET":
			if target, ok := targetsByID[rule.TargetID]; ok {
				endpoint := toTargetEndpoint(target)
				config.Upstream = RuleUpstreamConfig{Type: "TARGET", Target: &endpoint}
			}
		case "TARGET_GROUP":
			if group, ok := targetGroupsByID[rule.TargetGroupID]; ok {
				if !targetGroupSchedulerSupportedByOSS(group.Scheduler) {
					continue
				}
				config.Upstream = RuleUpstreamConfig{Type: "TARGET_GROUP", Scheduler: normalizeTargetGroupScheduler(group.Scheduler), TargetGroup: toTargetPriorityBuckets(group, targetsByID)}
			}
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func toAgentPortSegments(segments []repo.InboundBindingPortSegmentRecord) []agent.PortSegmentConfig {
	if len(segments) == 0 {
		return nil
	}
	configs := make([]agent.PortSegmentConfig, 0, len(segments))
	for _, segment := range segments {
		configs = append(configs, agent.PortSegmentConfig{StartPort: segment.StartPort, EndPort: segment.EndPort})
	}
	return configs
}

func openCoreRuleRecordSupported(rule repo.RuleRecord) bool {
	if defaultForwardingType(rule.ForwardingType) != string(domain.ForwardingTypeDirect) {
		return false
	}
	switch domain.MatchType(rule.MatchType) {
	case domain.MatchTypeAnyInbound, domain.MatchTypeTLSSNI:
		return true
	default:
		return false
	}
}

func toTargetEndpoint(target repo.TargetRecord) TargetEndpoint {
	return TargetEndpoint{
		ID:      target.ID,
		Host:    target.Host,
		Port:    target.Port,
		Weight:  1,
		Enabled: target.Enabled,
	}
}

func toTargetPriorityBuckets(group repo.TargetGroupRecord, targetsByID map[string]repo.TargetRecord) []TargetPriorityBucket {
	bucketsByPriority := make(map[int][]TargetEndpoint)
	for _, member := range group.Members {
		target, ok := targetsByID[member.TargetID]
		if !ok {
			continue
		}
		endpoint := toTargetEndpoint(target)
		endpoint.Enabled = endpoint.Enabled && member.Enabled
		endpoint.Weight = member.Weight
		bucketsByPriority[member.Priority] = append(bucketsByPriority[member.Priority], endpoint)
	}
	priorities := make([]int, 0, len(bucketsByPriority))
	for priority := range bucketsByPriority {
		priorities = append(priorities, priority)
	}
	sort.Ints(priorities)
	buckets := make([]TargetPriorityBucket, 0, len(priorities))
	for _, priority := range priorities {
		buckets = append(buckets, TargetPriorityBucket{Priority: priority, Targets: bucketsByPriority[priority]})
	}
	return buckets
}
