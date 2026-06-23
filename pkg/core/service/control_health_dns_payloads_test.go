package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestHealthCheckPayloadPreservesTargetGroupScopeWithoutPlaceholderTarget(t *testing.T) {
	payload := toHealthCheckPayload(repo.HealthCheckRecord{
		ID:             "health_1",
		OrganizationID: "org_1",
		Targets: []repo.HealthCheckTargetRecord{{
			ID:            "health_target_placeholder",
			ScopeType:     "TARGET_GROUP",
			TargetGroupID: "target_group_1",
		}, {
			ID:         "health_target_1",
			ScopeType:  "TARGET_GROUP",
			TargetID:   "target_1",
			TargetName: "current",
			TargetHost: "198.51.100.20",
			TargetPort: 443,
		}},
	})

	if payload.TargetScope.Type != "TARGET_GROUP" || payload.TargetScope.TargetGroupID != "target_group_1" {
		t.Fatalf("expected target-group scope to be exposed separately, got %#v", payload.TargetScope)
	}
	if len(payload.Targets) != 1 {
		t.Fatalf("expected only expanded target binding, got %#v", payload.Targets)
	}
	if payload.Targets[0].TargetID != "target_1" {
		t.Fatalf("expected expanded target binding, got %#v", payload.Targets[0])
	}
}

func TestListHealthChecksBatchesLatestResults(t *testing.T) {
	store := &healthDNSTestStore{
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Name:           "first",
		}, {
			ID:             "health_2",
			OrganizationID: "org_1",
			Name:           "second",
		}},
		results: []repo.HealthResultRecord{{
			ID:                  "result_1",
			OrganizationID:      "org_1",
			HealthCheckID:       "health_1",
			HealthCheckTargetID: "health_target_1",
			MonitorID:           "monitor_1",
			Status:              "ONLINE",
			ObservedAt:          "2026-06-22T00:00:00Z",
			CreatedAt:           "2026-06-22T00:00:00Z",
		}, {
			ID:                  "result_2",
			OrganizationID:      "org_1",
			HealthCheckID:       "health_2",
			HealthCheckTargetID: "health_target_2",
			MonitorID:           "monitor_1",
			Status:              "OFFLINE",
			ObservedAt:          "2026-06-22T00:01:00Z",
			CreatedAt:           "2026-06-22T00:01:00Z",
		}},
	}
	control := NewControlService(store)

	payloads, err := control.ListHealthChecks(context.Background(), healthDNSTestIdentity(string(domain.PermissionHealthChecksRead)))
	if err != nil {
		t.Fatalf("list health checks: %v", err)
	}
	if store.latestHealthBatchCalls != 1 || store.latestHealthSingleCalls != 0 {
		t.Fatalf("expected one batch query and no per-check queries, got batch=%d single=%d", store.latestHealthBatchCalls, store.latestHealthSingleCalls)
	}
	if len(payloads) != 2 || len(payloads[0].LatestResults)+len(payloads[1].LatestResults) != 2 {
		t.Fatalf("expected latest results on both payloads, got %#v", payloads)
	}
}

func TestHealthResultPayloadIncludesZeroLatency(t *testing.T) {
	payloads := toHealthResultPayloads([]repo.HealthResultRecord{{
		ID:                  "result_1",
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		MonitorID:           "monitor_1",
		TargetID:            "target_1",
		Status:              "PASS",
		LatencyMS:           0,
		ObservedAt:          "2026-06-22T00:00:00Z",
		CreatedAt:           "2026-06-22T00:00:00Z",
	}})

	data, err := json.Marshal(payloads[0])
	if err != nil {
		t.Fatalf("marshal health result payload: %v", err)
	}
	if !strings.Contains(string(data), `"latency_ms":0`) {
		t.Fatalf("expected zero latency to be emitted, got %s", string(data))
	}
}
