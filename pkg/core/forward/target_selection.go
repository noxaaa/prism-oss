package forward

import (
	"hash/fnv"
	"net"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

func (supervisor *Supervisor) selectTarget(rule agent.RuleConfig, source net.Addr) (agent.TargetEndpoint, bool) {
	if rule.Upstream.Type == "TARGET" && rule.Upstream.Target != nil && rule.Upstream.Target.Enabled {
		return *rule.Upstream.Target, true
	}
	if rule.Upstream.Type == "TARGET_GROUP" {
		return supervisor.selectTargetGroupEndpoint(rule, source)
	}
	return agent.TargetEndpoint{}, false
}

func (supervisor *Supervisor) selectTCPTarget(rule agent.RuleConfig, source net.Addr) (agent.TargetEndpoint, bool, bool) {
	if rule.Upstream.Type == "TARGET_GROUP" && strings.EqualFold(strings.TrimSpace(rule.Upstream.Scheduler), "LEAST_LOAD") {
		return supervisor.metrics.reserveLeastLoadTargetTCPDial(rule.ID, enabledLeastLoadTargetGroupCandidates(rule))
	}
	target, ok := supervisor.selectTarget(rule, source)
	if !ok {
		return agent.TargetEndpoint{}, false, false
	}
	return target, supervisor.metrics.reserveTargetTCPDial(rule.ID, target.ID), true
}

func (supervisor *Supervisor) selectUDPTarget(rule agent.RuleConfig, source net.Addr) (agent.TargetEndpoint, bool, bool) {
	if rule.Upstream.Type == "TARGET_GROUP" && strings.EqualFold(strings.TrimSpace(rule.Upstream.Scheduler), "LEAST_LOAD") {
		return supervisor.metrics.reserveLeastLoadTargetUDPSession(rule.ID, enabledLeastLoadTargetGroupCandidates(rule))
	}
	target, ok := supervisor.selectTarget(rule, source)
	if !ok {
		return agent.TargetEndpoint{}, false, false
	}
	return target, false, true
}

func (supervisor *Supervisor) selectTargetGroupEndpoint(rule agent.RuleConfig, source net.Addr) (agent.TargetEndpoint, bool) {
	if strings.EqualFold(strings.TrimSpace(rule.Upstream.Scheduler), "LEAST_LOAD") {
		return supervisor.selectLeastLoadTargetGroupEndpoint(rule)
	}
	candidates := enabledTargetGroupCandidates(rule)
	if len(candidates) == 0 {
		return agent.TargetEndpoint{}, false
	}
	index := int(stableTargetHash(sourceIPKey(source), rule.ID, string(rule.Protocol)) % uint32(len(candidates)))
	return candidates[index], true
}

func (supervisor *Supervisor) selectLeastLoadTargetGroupEndpoint(rule agent.RuleConfig) (agent.TargetEndpoint, bool) {
	candidates := enabledLeastLoadTargetGroupCandidates(rule)
	if len(candidates) == 0 {
		return agent.TargetEndpoint{}, false
	}
	counts := supervisor.metrics.openConnectionsForTargets(rule.ID, candidates)
	best := candidates[0]
	bestConnections := counts[best.ID]
	for _, candidate := range candidates[1:] {
		candidateConnections := counts[candidate.ID]
		if candidateConnections*int64(best.Weight) < bestConnections*int64(candidate.Weight) {
			best = candidate
			bestConnections = candidateConnections
		}
	}
	return best, true
}

func enabledTargetGroupCandidates(rule agent.RuleConfig) []agent.TargetEndpoint {
	bestPrioritySet := false
	bestPriority := 0
	var candidates []agent.TargetEndpoint
	for _, bucket := range rule.Upstream.TargetGroup {
		enabledTargets := make([]agent.TargetEndpoint, 0, len(bucket.Targets))
		for _, target := range bucket.Targets {
			if target.Enabled {
				enabledTargets = append(enabledTargets, target)
			}
		}
		if len(enabledTargets) == 0 {
			continue
		}
		if !bestPrioritySet || bucket.Priority < bestPriority {
			bestPrioritySet = true
			bestPriority = bucket.Priority
			candidates = enabledTargets
		}
	}
	return candidates
}

func enabledLeastLoadTargetGroupCandidates(rule agent.RuleConfig) []agent.TargetEndpoint {
	bestPrioritySet := false
	bestPriority := 0
	var candidates []agent.TargetEndpoint
	for _, bucket := range rule.Upstream.TargetGroup {
		enabledTargets := make([]agent.TargetEndpoint, 0, len(bucket.Targets))
		for _, target := range bucket.Targets {
			if target.Enabled && target.Weight > 0 {
				enabledTargets = append(enabledTargets, target)
			}
		}
		if len(enabledTargets) == 0 {
			continue
		}
		if !bestPrioritySet || bucket.Priority < bestPriority {
			bestPrioritySet = true
			bestPriority = bucket.Priority
			candidates = enabledTargets
		}
	}
	return candidates
}

func activeTargetGroupCandidates(rule agent.RuleConfig) []agent.TargetEndpoint {
	if strings.EqualFold(strings.TrimSpace(rule.Upstream.Scheduler), "LEAST_LOAD") {
		return enabledLeastLoadTargetGroupCandidates(rule)
	}
	return enabledTargetGroupCandidates(rule)
}

func sourceIPKey(address net.Addr) string {
	if address == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return address.String()
	}
	return host
}

func stableTargetHash(parts ...string) uint32 {
	hash := fnv.New32a()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hash.Sum32()
}
