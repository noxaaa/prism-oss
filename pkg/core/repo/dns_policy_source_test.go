package repo

import (
	"os"
	"strings"
	"testing"
)

func TestFindDNSInstanceByIDFiltersDeletedManagedRecords(t *testing.T) {
	source, err := os.ReadFile("dns_policy.go")
	if err != nil {
		t.Fatalf("read dns policy repo source: %v", err)
	}
	text := string(source)
	start := strings.Index(text, "func (store *PostgresStore) FindDNSInstanceByID")
	if start < 0 {
		t.Fatalf("FindDNSInstanceByID not found")
	}
	end := strings.Index(text[start:], "func (store *PostgresStore) CreateDNSInstance")
	if end < 0 {
		t.Fatalf("CreateDNSInstance not found after FindDNSInstanceByID")
	}
	body := text[start : start+end]
	for _, required := range []string{
		"JOIN dns_managed_records",
		"dns_managed_records.deleted_at IS NULL",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("FindDNSInstanceByID must ignore instances under soft-deleted managed records; missing %q in:\n%s", required, body)
		}
	}
}

func TestClearDNSManagedRecordActiveInstanceMarksRecordPending(t *testing.T) {
	source, err := os.ReadFile("dns_policy.go")
	if err != nil {
		t.Fatalf("read dns policy repo source: %v", err)
	}
	text := string(source)
	start := strings.Index(text, "func (store *PostgresStore) ClearDNSManagedRecordActiveInstance")
	if start < 0 {
		t.Fatalf("ClearDNSManagedRecordActiveInstance not found")
	}
	end := strings.Index(text[start:], "func (store *PostgresStore) UpdateDNSInstanceEvaluation")
	if end < 0 {
		t.Fatalf("UpdateDNSInstanceEvaluation not found after ClearDNSManagedRecordActiveInstance")
	}
	body := text[start : start+end]
	for _, required := range []string{
		"last_evaluation_status = 'PENDING'",
		"last_evaluation_error = ''",
		"STALE_ACTIVE_INSTANCE_CLEARED",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("clearing an active DNS instance must mark the record pending; missing %q in:\n%s", required, body)
		}
	}
}
