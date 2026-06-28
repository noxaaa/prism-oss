package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestManualNodePayloadOmitsEnrollmentProfile(t *testing.T) {
	payload := toNodePayload(repo.NodeRecord{ID: "node_1", Name: "manual", Status: "OFFLINE"})
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal node payload: %v", err)
	}
	if strings.Contains(string(encoded), "enrollment_profile") {
		t.Fatalf("expected manual node payload to omit enrollment_profile, got %s", encoded)
	}
}

func TestUniqueEnrollmentNodeNameKeepsSuffixWithinLimit(t *testing.T) {
	nodeID := "12345678-1234-1234-1234-123456789abc"
	base := strings.Repeat("a", 120)
	name := uniqueEnrollmentNodeName(base, []repo.NodeRecord{{Name: base}}, nodeID)
	if len(name) > 120 {
		t.Fatalf("expected suffixed name to fit 120 bytes, got %d bytes: %q", len(name), name)
	}
	if !strings.HasSuffix(name, "-12345678") {
		t.Fatalf("expected suffixed name to preserve short node id suffix, got %q", name)
	}
}
