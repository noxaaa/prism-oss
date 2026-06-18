package forward

import (
	"sort"
	"time"

	"github.com/noxaaa/prism-oss/internal/agent"
)

type Metrics struct {
	TCPConnections int64
	UDPPackets     int64
	UploadBytes    int64
	DownloadBytes  int64
}

type targetMetricKey struct {
	ruleID   string
	targetID string
}

type targetMetricCounter struct {
	tcpConnections int64
	udpPackets     int64
	uploadBytes    int64
	downloadBytes  int64
	latencyMS      int64
}

type targetMetricSnapshot struct {
	TCPConnections int64
	UDPPackets     int64
	UploadBytes    int64
	DownloadBytes  int64
	LatencyMS      int64
}

func newMetricsCounter() *metricsCounter {
	return &metricsCounter{
		targets:        map[targetMetricKey]*targetMetricCounter{},
		lastTargets:    map[targetMetricKey]targetMetricSnapshot{},
		activeTargets:  map[targetMetricKey]bool{},
		lastSnapshotAt: time.Now(),
	}
}

func (metrics *metricsCounter) snapshot() Metrics {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	return Metrics{
		TCPConnections: metrics.tcpConnections,
		UDPPackets:     metrics.udpPackets,
		UploadBytes:    metrics.uploadBytes,
		DownloadBytes:  metrics.downloadBytes,
	}
}

func (metrics *metricsCounter) setActiveTargets(rules []agent.RuleConfig) {
	activeTargets := activeTargetKeys(rules)
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.activeTargets = activeTargets
	metrics.activeApplied = true
	metrics.pruneInactiveTargetsLocked()
}

func activeTargetKeys(rules []agent.RuleConfig) map[targetMetricKey]bool {
	activeTargets := make(map[targetMetricKey]bool)
	for _, rule := range rules {
		if !rule.Enabled || rule.ID == "" {
			continue
		}
		switch rule.Upstream.Type {
		case "TARGET":
			if rule.Upstream.Target != nil && rule.Upstream.Target.Enabled && rule.Upstream.Target.ID != "" {
				activeTargets[targetMetricKey{ruleID: rule.ID, targetID: rule.Upstream.Target.ID}] = true
			}
		case "TARGET_GROUP":
			for _, target := range enabledTargetGroupCandidates(rule) {
				if target.ID != "" {
					activeTargets[targetMetricKey{ruleID: rule.ID, targetID: target.ID}] = true
				}
			}
		}
	}
	return activeTargets
}

func (metrics *metricsCounter) pruneInactiveTargetsLocked() {
	if !metrics.activeApplied {
		return
	}
	for key, target := range metrics.targets {
		if metrics.activeTargets[key] {
			continue
		}
		if target.tcpConnections <= 0 {
			delete(metrics.targets, key)
			continue
		}
		target.udpPackets = 0
		target.uploadBytes = 0
		target.downloadBytes = 0
		target.latencyMS = 0
	}
	for key := range metrics.lastTargets {
		if !metrics.activeTargets[key] {
			delete(metrics.lastTargets, key)
		}
	}
}

func (metrics *metricsCounter) copyReportableTargetSnapshotsLocked() map[targetMetricKey]targetMetricSnapshot {
	snapshots := make(map[targetMetricKey]targetMetricSnapshot, len(metrics.targets))
	for key, target := range metrics.targets {
		if metrics.activeApplied && !metrics.activeTargets[key] {
			continue
		}
		snapshots[key] = targetMetricSnapshot{
			TCPConnections: target.tcpConnections,
			UDPPackets:     target.udpPackets,
			UploadBytes:    target.uploadBytes,
			DownloadBytes:  target.downloadBytes,
			LatencyMS:      target.latencyMS,
		}
	}
	return snapshots
}

func (metrics *metricsCounter) agentPayload(now time.Time) agent.MetricsPayload {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.pruneInactiveTargetsLocked()
	current := Metrics{
		TCPConnections: metrics.tcpConnections,
		UDPPackets:     metrics.udpPackets,
		UploadBytes:    metrics.uploadBytes,
		DownloadBytes:  metrics.downloadBytes,
	}
	currentTargets := metrics.copyReportableTargetSnapshotsLocked()
	payload := agent.MetricsPayload{
		TCPConnections: current.TCPConnections,
		UploadBytes:    current.UploadBytes,
		DownloadBytes:  current.DownloadBytes,
		Targets:        make([]agent.TargetMetricsPayload, 0, len(currentTargets)),
	}
	keys := make([]targetMetricKey, 0, len(currentTargets))
	for key := range currentTargets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left int, right int) bool {
		if keys[left].ruleID != keys[right].ruleID {
			return keys[left].ruleID < keys[right].ruleID
		}
		return keys[left].targetID < keys[right].targetID
	})
	if !metrics.lastSnapshotAt.IsZero() {
		elapsed := now.Sub(metrics.lastSnapshotAt).Seconds()
		if elapsed > 0 {
			payload.UDPPacketsPerSecond = int64(float64(current.UDPPackets-metrics.lastSnapshot.UDPPackets) / elapsed)
			payload.BandwidthBps = int64(float64(current.UploadBytes+current.DownloadBytes-metrics.lastSnapshot.UploadBytes-metrics.lastSnapshot.DownloadBytes) * 8 / elapsed)
			for _, key := range keys {
				target := currentTargets[key]
				lastTarget := metrics.lastTargets[key]
				payload.Targets = append(payload.Targets, agent.TargetMetricsPayload{
					RuleID:              key.ruleID,
					TargetID:            key.targetID,
					TCPConnections:      target.TCPConnections,
					UDPPacketsPerSecond: int64(float64(target.UDPPackets-lastTarget.UDPPackets) / elapsed),
					BandwidthBps:        int64(float64(target.UploadBytes+target.DownloadBytes-lastTarget.UploadBytes-lastTarget.DownloadBytes) * 8 / elapsed),
					UploadBytes:         target.UploadBytes,
					DownloadBytes:       target.DownloadBytes,
					LatencyMS:           target.LatencyMS,
				})
			}
		}
	}
	if len(payload.Targets) == 0 {
		for _, key := range keys {
			target := currentTargets[key]
			payload.Targets = append(payload.Targets, agent.TargetMetricsPayload{
				RuleID:         key.ruleID,
				TargetID:       key.targetID,
				TCPConnections: target.TCPConnections,
				UploadBytes:    target.UploadBytes,
				DownloadBytes:  target.DownloadBytes,
				LatencyMS:      target.LatencyMS,
			})
		}
	}
	metrics.lastSnapshot = current
	metrics.lastTargets = currentTargets
	metrics.lastSnapshotAt = now
	return payload
}

func (metrics *metricsCounter) addTCPConnection(delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.tcpConnections += delta
}

func (metrics *metricsCounter) addUDP(delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.udpPackets += delta
}

func (metrics *metricsCounter) addUpload(delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.uploadBytes += delta
}

func (metrics *metricsCounter) addDownload(delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.downloadBytes += delta
}

func (metrics *metricsCounter) targetLocked(ruleID string, targetID string) (*targetMetricCounter, bool) {
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	if metrics.activeApplied && !metrics.activeTargets[key] {
		return nil, false
	}
	target := metrics.targets[key]
	if target == nil {
		target = &targetMetricCounter{}
		metrics.targets[key] = target
	}
	return target, true
}

func (metrics *metricsCounter) addTargetTCPConnection(ruleID string, targetID string, delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	target := metrics.targets[key]
	if target == nil {
		if delta < 0 || (metrics.activeApplied && !metrics.activeTargets[key]) {
			return
		}
		target = &targetMetricCounter{}
		metrics.targets[key] = target
	}
	target.tcpConnections += delta
	if target.tcpConnections < 0 {
		target.tcpConnections = 0
	}
	if metrics.activeApplied && !metrics.activeTargets[key] && target.tcpConnections <= 0 {
		delete(metrics.targets, key)
	}
}

func (metrics *metricsCounter) addTargetUDP(ruleID string, targetID string, delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if target, ok := metrics.targetLocked(ruleID, targetID); ok {
		target.udpPackets += delta
	}
}

func (metrics *metricsCounter) addTargetUpload(ruleID string, targetID string, delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if target, ok := metrics.targetLocked(ruleID, targetID); ok {
		target.uploadBytes += delta
	}
}

func (metrics *metricsCounter) addTargetDownload(ruleID string, targetID string, delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if target, ok := metrics.targetLocked(ruleID, targetID); ok {
		target.downloadBytes += delta
	}
}

func (metrics *metricsCounter) recordTargetLatency(ruleID string, targetID string, latency time.Duration) {
	latencyMS := latency.Milliseconds()
	if latency > 0 && latencyMS == 0 {
		latencyMS = 1
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if target, ok := metrics.targetLocked(ruleID, targetID); ok {
		target.latencyMS = latencyMS
	}
}
