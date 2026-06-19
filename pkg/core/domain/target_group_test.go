package domain

import "testing"

func TestTargetGroupSelectsLowestNumericPriorityWithHealthyTargets(t *testing.T) {
	group := TargetGroup{
		Members: []TargetGroupMember{
			{TargetID: "backup", Priority: 20, Enabled: true, Healthy: true},
			{TargetID: "primary", Priority: 10, Enabled: true, Healthy: true},
		},
	}

	target, ok := group.Select("203.0.113.4", "rule_1", ProtocolTCP)
	if !ok {
		t.Fatalf("expected target")
	}
	if target.TargetID != "primary" {
		t.Fatalf("expected primary target, got %s", target.TargetID)
	}
}

func TestTargetGroupFallsBackToNextPriorityWhenPrimaryIsUnhealthy(t *testing.T) {
	group := TargetGroup{
		Members: []TargetGroupMember{
			{TargetID: "primary", Priority: 10, Enabled: true, Healthy: false},
			{TargetID: "backup", Priority: 20, Enabled: true, Healthy: true},
		},
	}

	target, ok := group.Select("203.0.113.4", "rule_1", ProtocolTCP)
	if !ok {
		t.Fatalf("expected target")
	}
	if target.TargetID != "backup" {
		t.Fatalf("expected backup target, got %s", target.TargetID)
	}
}

func TestTargetGroupIphashIsStableWithinPriority(t *testing.T) {
	group := TargetGroup{
		Members: []TargetGroupMember{
			{TargetID: "a", Priority: 10, Enabled: true, Healthy: true},
			{TargetID: "b", Priority: 10, Enabled: true, Healthy: true},
			{TargetID: "c", Priority: 10, Enabled: true, Healthy: true},
		},
	}

	first, ok := group.Select("203.0.113.4", "rule_1", ProtocolTCP)
	if !ok {
		t.Fatalf("expected first target")
	}

	for i := 0; i < 20; i++ {
		next, ok := group.Select("203.0.113.4", "rule_1", ProtocolTCP)
		if !ok {
			t.Fatalf("expected target at iteration %d", i)
		}
		if next.TargetID != first.TargetID {
			t.Fatalf("expected stable target %s, got %s", first.TargetID, next.TargetID)
		}
	}
}
