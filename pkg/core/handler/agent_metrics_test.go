package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/edition"
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
			DiskUsedBytes:        100,
			DiskTotalBytes:       200,
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
	for _, key := range []string{"disk_used_bytes", "disk_total_bytes"} {
		if _, ok := withoutHostDetails[key]; !ok {
			t.Fatalf("expected monitor metrics payload to retain realtime disk metric %q: %#v", key, withoutHostDetails)
		}
	}
}

func TestOrganizationNodeMetricsStreamSendsVisibleNodeSnapshots(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions: []string{
			string(domain.PermissionNodesRead),
			string(domain.PermissionNodesManage),
			string(domain.PermissionTrafficReadAll),
		},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Metrics Stream Group")
	node := createOSSNodeViaAPI(t, server, token, group.ID, "Metrics Stream Node")
	server.agentStates.UpdateMetrics(bootstrapResponse.Data.Organization.ID, "NODE", node.ID, agent.MetricsPayload{
		CPUPercent:     42.5,
		RAMUsedBytes:   128,
		RAMTotalBytes:  256,
		DiskUsedBytes:  512,
		DiskTotalBytes: 1024,
	})
	server.agentStates.UpdateMetrics("other-org", "NODE", "other-node", agent.MetricsPayload{CPUPercent: 99})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/nodes/metrics/stream?once=true", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected org node metrics stream 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: metrics") || !strings.Contains(body, `"node_id":"`+node.ID+`"`) || !strings.Contains(body, `"cpu_percent":42.5`) {
		t.Fatalf("expected visible node metrics event, got %s", body)
	}
	if strings.Contains(body, "other-node") {
		t.Fatalf("expected organization metrics stream to filter other organizations, got %s", body)
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
