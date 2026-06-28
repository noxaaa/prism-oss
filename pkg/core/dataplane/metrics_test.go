package dataplane

import "testing"

func TestAddManagedTrafficSnapshotAggregatesSameRule(t *testing.T) {
	values := map[string]managedTrafficSnapshot{}
	addManagedTrafficSnapshot(values, "rule_range", managedTrafficSnapshot{
		UploadBytes:         10,
		DownloadBytes:       20,
		TCPConnectionEvents: 1,
		UDPPackets:          2,
	})
	addManagedTrafficSnapshot(values, "rule_range", managedTrafficSnapshot{
		UploadBytes:         30,
		DownloadBytes:       40,
		TCPConnectionEvents: 3,
		UDPPackets:          4,
	})

	got := values["rule_range"]
	if got.UploadBytes != 40 || got.DownloadBytes != 60 || got.TCPConnectionEvents != 4 || got.UDPPackets != 6 {
		t.Fatalf("expected snapshots to aggregate by rule id, got %#v", got)
	}
}
