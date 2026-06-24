package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestNodeAndMonitorPayloadGroupIDsEncodeAsEmptyArrays(t *testing.T) {
	payloads := []any{
		toNodePayload(repo.NodeRecord{}),
		toNodePayload(repo.NodeRecord{GroupIDs: []string{}}),
		toMonitorPayload(repo.MonitorRecord{}),
		toMonitorPayload(repo.MonitorRecord{GroupIDs: []string{}}),
	}
	for _, payload := range payloads {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		text := string(data)
		if strings.Contains(text, `"group_ids":null`) {
			t.Fatalf("group_ids must encode as an empty array, got %s", text)
		}
		if !strings.Contains(text, `"group_ids":[]`) {
			t.Fatalf("expected group_ids empty array in payload, got %s", text)
		}
	}
}
