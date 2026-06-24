package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

func TestWriteMetricsSSEGatesHostDetails(t *testing.T) {
	state := AgentMetricsState{
		Status:     "ONLINE",
		LastSeenAt: "2026-06-24T00:00:00Z",
		Metrics: agent.MetricsPayload{
			CPUPercent:           12.5,
			CPUModel:             "Test CPU",
			CPULogicalCores:      8,
			CPUPhysicalCores:     4,
			OSName:               "linux",
			OSVersion:            "6.0",
			KernelVersion:        "6.1.0",
			Architecture:         "amd64",
			VirtualizationSystem: "kvm",
			VirtualizationRole:   "guest",
		},
	}

	withHostDetails := writeMetricsPayloadForTest(t, state, true)
	for _, key := range []string{"cpu_model", "cpu_logical_cores", "cpu_physical_cores", "os_name", "os_version", "kernel_version", "architecture", "virtualization_system", "virtualization_role"} {
		if _, ok := withHostDetails[key]; !ok {
			t.Fatalf("expected node metrics payload to include %q: %#v", key, withHostDetails)
		}
	}

	withoutHostDetails := writeMetricsPayloadForTest(t, state, false)
	for _, key := range []string{"cpu_model", "cpu_logical_cores", "cpu_physical_cores", "os_name", "os_version", "kernel_version", "architecture", "virtualization_system", "virtualization_role"} {
		if _, ok := withoutHostDetails[key]; ok {
			t.Fatalf("expected monitor metrics payload to omit %q: %#v", key, withoutHostDetails)
		}
	}
	if _, ok := withoutHostDetails["cpu_percent"]; !ok {
		t.Fatalf("expected monitor metrics payload to retain realtime CPU percentage")
	}
}

func writeMetricsPayloadForTest(t *testing.T, state AgentMetricsState, includeHostDetails bool) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	if !writeMetricsSSE(recorder, state, includeHostDetails) {
		t.Fatalf("write metrics SSE failed")
	}
	body := recorder.Body.String()
	dataPrefix := "data: "
	start := strings.Index(body, dataPrefix)
	if start < 0 {
		t.Fatalf("missing SSE data line: %q", body)
	}
	data := strings.TrimSpace(body[start+len(dataPrefix):])
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("decode metrics payload: %v body=%q", err, body)
	}
	return payload
}
