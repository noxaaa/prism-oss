package dataplane

import (
	"strconv"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

type managedTrafficSnapshot struct {
	UploadBytes         int64
	DownloadBytes       int64
	TCPConnectionEvents int64
	UDPPackets          int64
}

func addManagedTrafficSnapshot(values map[string]managedTrafficSnapshot, ruleID string, snapshot managedTrafficSnapshot) {
	current := values[ruleID]
	current.UploadBytes += snapshot.UploadBytes
	current.DownloadBytes += snapshot.DownloadBytes
	current.TCPConnectionEvents += snapshot.TCPConnectionEvents
	current.UDPPackets += snapshot.UDPPackets
	values[ruleID] = current
}

func managedTrafficDeltas(current map[string]managedTrafficSnapshot, last map[string]managedTrafficSnapshot) []agent.RuleTrafficDelta {
	if len(current) == 0 {
		return nil
	}
	deltas := make([]agent.RuleTrafficDelta, 0, len(current))
	for ruleID, snapshot := range current {
		previous := last[ruleID]
		delta := agent.RuleTrafficDelta{
			RuleID:         ruleID,
			UploadBytes:    positiveManagedDelta(snapshot.UploadBytes, previous.UploadBytes),
			DownloadBytes:  positiveManagedDelta(snapshot.DownloadBytes, previous.DownloadBytes),
			TCPConnections: positiveManagedDelta(snapshot.TCPConnectionEvents, previous.TCPConnectionEvents),
			UDPPackets:     positiveManagedDelta(snapshot.UDPPackets, previous.UDPPackets),
		}
		if delta.UploadBytes != 0 || delta.DownloadBytes != 0 || delta.TCPConnections != 0 || delta.UDPPackets != 0 {
			deltas = append(deltas, delta)
		}
	}
	return deltas
}

func mergeMetricsPayloads(payloads ...agent.MetricsPayload) agent.MetricsPayload {
	if len(payloads) == 0 {
		return agent.MetricsPayload{}
	}
	result := payloads[0]
	for _, payload := range payloads[1:] {
		result.TrafficDeltas = append(result.TrafficDeltas, payload.TrafficDeltas...)
		result.Targets = append(result.Targets, payload.Targets...)
	}
	result.TrafficDeltas = mergeDataplaneTrafficDeltas(result.TrafficDeltas)
	return result
}

func mergeDataplaneTrafficDeltas(deltas []agent.RuleTrafficDelta) []agent.RuleTrafficDelta {
	byRule := map[string]agent.RuleTrafficDelta{}
	order := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		if strings.TrimSpace(delta.RuleID) == "" {
			continue
		}
		current, ok := byRule[delta.RuleID]
		if !ok {
			current.RuleID = delta.RuleID
			order = append(order, delta.RuleID)
		}
		current.UploadBytes += delta.UploadBytes
		current.DownloadBytes += delta.DownloadBytes
		current.TCPConnections += delta.TCPConnections
		current.UDPPackets += delta.UDPPackets
		byRule[delta.RuleID] = current
	}
	merged := make([]agent.RuleTrafficDelta, 0, len(order))
	for _, ruleID := range order {
		merged = append(merged, byRule[ruleID])
	}
	return merged
}

func positiveManagedDelta(current int64, last int64) int64 {
	if current < 0 {
		return 0
	}
	if current < last {
		return current
	}
	if current == last {
		return 0
	}
	return current - last
}

func csvHeaderIndexes(header []string) map[string]int {
	indexes := make(map[string]int, len(header))
	for index, value := range header {
		indexes[strings.TrimSpace(value)] = index
	}
	return indexes
}

func csvValue(row []string, indexes map[string]int, name string) string {
	index, ok := indexes[name]
	if !ok || index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
