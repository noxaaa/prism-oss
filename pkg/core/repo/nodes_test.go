package repo

import (
	"context"
	"strings"
	"testing"
)

func TestRecordNodeConfigAckGatesFailedDataplaneFieldsByDesiredVersion(t *testing.T) {
	executor := &recordingDBExecutor{affected: 1}
	store := &PostgresStore{db: executor}

	err := store.RecordNodeConfigAck(context.Background(), "org_1", "node_1", NodeConfigAckRecord{
		ConfigVersion:   7,
		Status:          "FAILED",
		ErrorMessage:    "stale failure",
		DataplaneStatus: "FAILED",
		DataplaneError:  "stale dataplane failure",
	}, "now")
	if err != nil {
		t.Fatalf("record failed ack: %v", err)
	}
	if len(executor.execs) != 1 {
		t.Fatalf("expected one node update, got %d", len(executor.execs))
	}
	query := executor.execs[0].query
	for _, required := range []string{
		"dataplane_status = CASE",
		"dataplane_error = CASE",
		"WHEN desired_config_version <= ? THEN ?",
		"ELSE dataplane_status",
		"ELSE dataplane_error",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("failed ack dataplane fields must be version-gated; missing %q in query:\n%s", required, query)
		}
	}
}

func TestRecordNodeConfigAckGatesAppliedDataplaneFieldsByDesiredVersion(t *testing.T) {
	executor := &recordingDBExecutor{affected: 1}
	store := &PostgresStore{db: executor}

	err := store.RecordNodeConfigAck(context.Background(), "org_1", "node_1", NodeConfigAckRecord{
		ConfigVersion:     7,
		Status:            "APPLIED",
		DataplaneStatus:   "HEALTHY",
		DataplaneLastHash: "hash_7",
	}, "now")
	if err != nil {
		t.Fatalf("record applied ack: %v", err)
	}
	if len(executor.execs) != 1 {
		t.Fatalf("expected one node update, got %d", len(executor.execs))
	}
	query := executor.execs[0].query
	for _, required := range []string{
		"dataplane_status = CASE",
		"dataplane_error = CASE",
		"dataplane_last_hash = CASE",
		"dataplane_last_applied_at = CASE",
		"WHEN desired_config_version <= ? THEN ?",
		"ELSE dataplane_status",
		"ELSE dataplane_error",
		"ELSE dataplane_last_hash",
		"ELSE dataplane_last_applied_at",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("applied ack dataplane fields must be version-gated; missing %q in query:\n%s", required, query)
		}
	}
}
