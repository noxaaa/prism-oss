package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type trafficSpoolFile struct {
	Pending        []RuleTrafficDelta `json:"pending,omitempty"`
	InFlightID     string             `json:"in_flight_id,omitempty"`
	InFlightDeltas []RuleTrafficDelta `json:"in_flight_deltas,omitempty"`
	UpdatedAt      string             `json:"updated_at"`
	SchemaVersion  string             `json:"schema_version"`
}

const trafficSpoolSchemaVersion = "agent_traffic_spool.v1"

func (runtime *NodeRuntime) queueTrafficDeltas(deltas []RuleTrafficDelta) {
	normalized := mergeTrafficDeltas(deltas)
	if len(normalized) == 0 {
		return
	}
	runtime.trafficMu.Lock()
	defer runtime.trafficMu.Unlock()
	runtime.loadTrafficSpoolLocked()
	runtime.trafficPending = mergeTrafficDeltas(append(runtime.trafficPending, normalized...))
	runtime.persistTrafficSpoolLocked()
}

func (runtime *NodeRuntime) attachTrafficReport(metrics *MetricsPayload) {
	runtime.trafficMu.Lock()
	defer runtime.trafficMu.Unlock()
	runtime.loadTrafficSpoolLocked()
	if runtime.trafficInFlightID == "" && len(runtime.trafficPending) > 0 {
		runtime.trafficInFlightID = newTrafficReportID()
		runtime.trafficInFlight = append([]RuleTrafficDelta(nil), runtime.trafficPending...)
		runtime.trafficPending = nil
		runtime.persistTrafficSpoolLocked()
	}
	if runtime.trafficInFlightID == "" || len(runtime.trafficInFlight) == 0 {
		return
	}
	metrics.TrafficReportID = runtime.trafficInFlightID
	metrics.TrafficDeltas = append([]RuleTrafficDelta(nil), runtime.trafficInFlight...)
}

func (runtime *NodeRuntime) acknowledgeTrafficReport(reportID string) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return
	}
	runtime.trafficMu.Lock()
	defer runtime.trafficMu.Unlock()
	runtime.loadTrafficSpoolLocked()
	if reportID != runtime.trafficInFlightID {
		return
	}
	runtime.trafficInFlightID = ""
	runtime.trafficInFlight = nil
	runtime.persistTrafficSpoolLocked()
}

func (runtime *NodeRuntime) loadTrafficSpoolLocked() {
	if runtime.trafficSpoolLoaded {
		return
	}
	runtime.trafficSpoolLoaded = true
	path := runtime.trafficSpoolPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var payload trafficSpoolFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if payload.SchemaVersion != "" && payload.SchemaVersion != trafficSpoolSchemaVersion {
		return
	}
	runtime.trafficPending = mergeTrafficDeltas(payload.Pending)
	runtime.trafficInFlightID = strings.TrimSpace(payload.InFlightID)
	runtime.trafficInFlight = mergeTrafficDeltas(payload.InFlightDeltas)
	if runtime.trafficInFlightID == "" || len(runtime.trafficInFlight) == 0 {
		runtime.trafficInFlightID = ""
		runtime.trafficInFlight = nil
	}
}

func (runtime *NodeRuntime) persistTrafficSpoolLocked() {
	path := runtime.trafficSpoolPath()
	if path == "" {
		return
	}
	if len(runtime.trafficPending) == 0 && len(runtime.trafficInFlight) == 0 {
		_ = os.Remove(path)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(trafficSpoolFile{
		Pending:        runtime.trafficPending,
		InFlightID:     runtime.trafficInFlightID,
		InFlightDeltas: runtime.trafficInFlight,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		SchemaVersion:  trafficSpoolSchemaVersion,
	})
	if err != nil {
		return
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".agent-traffic-spool-*")
	if err != nil {
		return
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return
	}
	if err := tempFile.Close(); err != nil {
		return
	}
	_ = os.Rename(tempPath, path)
	_ = os.Chmod(path, 0o600)
}

func (runtime *NodeRuntime) trafficSpoolPath() string {
	credentialFile := strings.TrimSpace(runtime.getAgentCredentialFile())
	if credentialFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(credentialFile), "agent-traffic-spool.json")
}

func mergeTrafficDeltas(deltas []RuleTrafficDelta) []RuleTrafficDelta {
	byRule := make(map[string]RuleTrafficDelta)
	order := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		ruleID := strings.TrimSpace(delta.RuleID)
		if ruleID == "" {
			continue
		}
		current, ok := byRule[ruleID]
		if !ok {
			order = append(order, ruleID)
			current.RuleID = ruleID
		}
		current.UploadBytes += maxInt64(delta.UploadBytes, 0)
		current.DownloadBytes += maxInt64(delta.DownloadBytes, 0)
		current.TCPConnections += maxInt64(delta.TCPConnections, 0)
		current.UDPPackets += maxInt64(delta.UDPPackets, 0)
		byRule[ruleID] = current
	}
	merged := make([]RuleTrafficDelta, 0, len(byRule))
	for _, ruleID := range order {
		delta := byRule[ruleID]
		if delta.UploadBytes == 0 && delta.DownloadBytes == 0 && delta.TCPConnections == 0 && delta.UDPPackets == 0 {
			continue
		}
		merged = append(merged, delta)
	}
	return merged
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func newTrafficReportID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return time.Now().UTC().Format("20060102150405.000000000")
}
