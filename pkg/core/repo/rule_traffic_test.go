package repo

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestRecordRuleTrafficReportRestrictsDeltasToAssignedNodeRules(t *testing.T) {
	executor := &recordingDBExecutor{affected: 1}
	store := &PostgresStore{db: executor}

	recorded, err := store.RecordRuleTrafficReport(
		context.Background(),
		"org_1",
		"node_1",
		RuleTrafficReportRecord{ReportID: "report_1"},
		[]RuleTrafficDeltaRecord{{RuleID: "rule_1", UploadBytes: 100}},
		"2026-01-01T00:00:00Z",
		func() string { return "id_1" },
	)
	if err != nil {
		t.Fatalf("record traffic report: %v", err)
	}
	if !recorded {
		t.Fatalf("expected report to be recorded")
	}
	if len(executor.execs) != 2 {
		t.Fatalf("expected report insert and counter upsert, got %d execs", len(executor.execs))
	}
	counter := executor.execs[1]
	for _, required := range []string{
		"FROM node_rule_traffic_assignments",
		"organization_id = ?",
		"node_id = ?",
		"rule_id = ?",
	} {
		if !strings.Contains(counter.query, required) {
			t.Fatalf("counter upsert must restrict rule deltas to rules assigned to the reporting node; missing %q in query:\n%s", required, counter.query)
		}
	}
	for _, forbidden := range []string{"node_group_members", "inbound_bindings"} {
		if strings.Contains(counter.query, forbidden) {
			t.Fatalf("counter upsert must not depend on current group membership; found %q in query:\n%s", forbidden, counter.query)
		}
	}
	if len(counter.args) < 3 || counter.args[len(counter.args)-3] != "org_1" || counter.args[len(counter.args)-2] != "node_1" || counter.args[len(counter.args)-1] != "rule_1" {
		t.Fatalf("counter upsert must bind assignment organization, node, and rule filters at the end, args=%#v", counter.args)
	}
}

func TestRecordNodeRuleTrafficAssignmentsUpsertsUniqueRules(t *testing.T) {
	executor := &recordingDBExecutor{affected: 1}
	store := &PostgresStore{db: executor}

	err := store.RecordNodeRuleTrafficAssignments(context.Background(), "org_1", "node_1", []string{"rule_1", "rule_1", " ", "rule_2"}, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("record node rule traffic assignments: %v", err)
	}
	if len(executor.execs) != 2 {
		t.Fatalf("expected two unique assignment upserts, got %d", len(executor.execs))
	}
	for _, exec := range executor.execs {
		if !strings.Contains(exec.query, "INSERT INTO node_rule_traffic_assignments") || !strings.Contains(exec.query, "ON CONFLICT (organization_id, node_id, rule_id)") {
			t.Fatalf("assignment must be recorded with an upsert, query:\n%s", exec.query)
		}
	}
	if executor.execs[0].args[2] != "rule_1" || executor.execs[1].args[2] != "rule_2" {
		t.Fatalf("unexpected assignment args: %#v %#v", executor.execs[0].args, executor.execs[1].args)
	}
}

type recordedExec struct {
	query string
	args  []any
}

type recordingDBExecutor struct {
	affected int64
	execs    []recordedExec
}

func (executor *recordingDBExecutor) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	executor.execs = append(executor.execs, recordedExec{query: query, args: append([]any(nil), args...)})
	return recordingSQLResult{affected: executor.affected}, nil
}

func (executor *recordingDBExecutor) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("QueryContext is not used by this test")
}

func (executor *recordingDBExecutor) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("QueryRowContext is not used by this test")
}

type recordingSQLResult struct {
	affected int64
}

func (result recordingSQLResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result recordingSQLResult) RowsAffected() (int64, error) {
	return result.affected, nil
}
