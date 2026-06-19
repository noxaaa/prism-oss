package service

import (
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

func TestMergeRuleTargetMetricsAggregatesRuleScopedSummaryAndLatestObservation(t *testing.T) {
	result := RuleDiagnosticsPayload{
		Targets: []RuleTargetDiagnosticsPayload{{TargetID: "target_1"}},
	}
	targetIndexes := map[string]int{"target_1": 0}

	mergeRuleTargetMetrics(&result, targetIndexes, "rule_1", AgentRuntimeMetricsInput{
		AgentID:    "node_newer",
		Status:     "ONLINE",
		LastSeenAt: "2026-06-16T12:00:00Z",
		Metrics: agent.MetricsPayload{Targets: []agent.TargetMetricsPayload{
			{RuleID: "rule_1", TargetID: "target_1", BandwidthBps: 40, UploadBytes: 5, DownloadBytes: 10, LatencyMS: 20},
		}},
	})
	mergeRuleTargetMetrics(&result, targetIndexes, "rule_1", AgentRuntimeMetricsInput{
		AgentID:    "node_older",
		Status:     "ONLINE",
		LastSeenAt: "2026-06-16T11:00:00Z",
		Metrics: agent.MetricsPayload{Targets: []agent.TargetMetricsPayload{
			{RuleID: "rule_1", TargetID: "target_1", BandwidthBps: 20, UploadBytes: 2, DownloadBytes: 3, LatencyMS: 15},
			{RuleID: "other_rule", TargetID: "target_1", BandwidthBps: 999, UploadBytes: 999, DownloadBytes: 999, LatencyMS: 1},
		}},
	})

	if result.BandwidthBps != 60 || result.UploadBytes != 7 || result.DownloadBytes != 13 {
		t.Fatalf("expected summary to include only matching rule target metrics, got %#v", result)
	}
	target := result.Targets[0]
	if target.LastSeenAt != "2026-06-16T12:00:00Z" {
		t.Fatalf("expected newest target observation to win, got %q", target.LastSeenAt)
	}
	if target.BandwidthBps == nil || *target.BandwidthBps != 60 || target.UploadBytes != 7 || target.DownloadBytes != 13 {
		t.Fatalf("expected target metrics to aggregate matching rule traffic, got %#v", target)
	}
	if target.LatencyMS == nil || *target.LatencyMS != 15 {
		t.Fatalf("expected lowest positive target latency to win, got %#v", target.LatencyMS)
	}
}
