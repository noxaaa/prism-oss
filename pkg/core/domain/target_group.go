package domain

import (
	"hash/fnv"
	"sort"
)

type TargetGroup struct {
	ID      string
	Members []TargetGroupMember
}

type TargetGroupMember struct {
	TargetID string
	Priority int
	Enabled  bool
	Healthy  bool
}

func (group TargetGroup) Select(sourceIP string, ruleID string, protocol Protocol) (TargetGroupMember, bool) {
	priorityMembers := group.availableMembersAtBestPriority()
	if len(priorityMembers) == 0 {
		return TargetGroupMember{}, false
	}
	sort.SliceStable(priorityMembers, func(i int, j int) bool {
		return priorityMembers[i].TargetID < priorityMembers[j].TargetID
	})
	index := stableIndex(sourceIP+"|"+ruleID+"|"+string(protocol), len(priorityMembers))
	return priorityMembers[index], true
}

func (group TargetGroup) availableMembersAtBestPriority() []TargetGroupMember {
	bestPriority := 0
	found := false
	for _, member := range group.Members {
		if !member.Enabled || !member.Healthy {
			continue
		}
		if !found || member.Priority < bestPriority {
			bestPriority = member.Priority
			found = true
		}
	}
	if !found {
		return nil
	}

	available := make([]TargetGroupMember, 0)
	for _, member := range group.Members {
		if member.Enabled && member.Healthy && member.Priority == bestPriority {
			available = append(available, member)
		}
	}
	return available
}

func stableIndex(input string, size int) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(input))
	return int(hash.Sum32() % uint32(size))
}
