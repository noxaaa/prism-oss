package agent

import (
	"path/filepath"
	"testing"
)

func TestTrafficSpoolPersistsInFlightReportUntilAck(t *testing.T) {
	credentialFile := filepath.Join(t.TempDir(), "agent-credential.json")
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Prism",
		ControlPlaneURL:     "http://127.0.0.1:8080",
		AgentCredential:     "credential",
		AgentCredentialFile: credentialFile,
	}, nil)

	runtime.queueTrafficDeltas([]RuleTrafficDelta{{RuleID: "rule_1", UploadBytes: 100, DownloadBytes: 50, TCPConnections: 1}})
	var first MetricsPayload
	runtime.attachTrafficReport(&first)
	if first.TrafficReportID == "" {
		t.Fatalf("expected traffic report id")
	}
	if len(first.TrafficDeltas) != 1 || first.TrafficDeltas[0].UploadBytes != 100 {
		t.Fatalf("expected queued traffic delta in report, got %#v", first.TrafficDeltas)
	}

	restarted := NewNodeRuntime(RuntimeConfig{
		AppName:             "Prism",
		ControlPlaneURL:     "http://127.0.0.1:8080",
		AgentCredential:     "credential",
		AgentCredentialFile: credentialFile,
	}, nil)
	var resent MetricsPayload
	restarted.attachTrafficReport(&resent)
	if resent.TrafficReportID != first.TrafficReportID {
		t.Fatalf("expected restart to resend same report id %q, got %q", first.TrafficReportID, resent.TrafficReportID)
	}
	if len(resent.TrafficDeltas) != 1 || resent.TrafficDeltas[0].DownloadBytes != 50 {
		t.Fatalf("expected restart to resend in-flight deltas, got %#v", resent.TrafficDeltas)
	}

	restarted.acknowledgeTrafficReport(resent.TrafficReportID)
	var afterAck MetricsPayload
	restarted.attachTrafficReport(&afterAck)
	if afterAck.TrafficReportID != "" || len(afterAck.TrafficDeltas) != 0 {
		t.Fatalf("expected ack to clear report, got %#v", afterAck)
	}
}

func TestTrafficSpoolQueuesNewDeltaBehindInFlightReport(t *testing.T) {
	credentialFile := filepath.Join(t.TempDir(), "agent-credential.json")
	runtime := NewNodeRuntime(RuntimeConfig{
		AppName:             "Prism",
		ControlPlaneURL:     "http://127.0.0.1:8080",
		AgentCredential:     "credential",
		AgentCredentialFile: credentialFile,
	}, nil)

	runtime.queueTrafficDeltas([]RuleTrafficDelta{{RuleID: "rule_1", UploadBytes: 100}})
	var first MetricsPayload
	runtime.attachTrafficReport(&first)
	runtime.queueTrafficDeltas([]RuleTrafficDelta{{RuleID: "rule_1", UploadBytes: 25}})

	var stillFirst MetricsPayload
	runtime.attachTrafficReport(&stillFirst)
	if stillFirst.TrafficReportID != first.TrafficReportID || stillFirst.TrafficDeltas[0].UploadBytes != 100 {
		t.Fatalf("expected in-flight report to remain first, got %#v", stillFirst)
	}

	runtime.acknowledgeTrafficReport(first.TrafficReportID)
	var second MetricsPayload
	runtime.attachTrafficReport(&second)
	if second.TrafficReportID == "" || second.TrafficReportID == first.TrafficReportID {
		t.Fatalf("expected new report id for queued delta, got first=%q second=%q", first.TrafficReportID, second.TrafficReportID)
	}
	if len(second.TrafficDeltas) != 1 || second.TrafficDeltas[0].UploadBytes != 25 {
		t.Fatalf("expected queued delta behind in-flight report, got %#v", second.TrafficDeltas)
	}
}
