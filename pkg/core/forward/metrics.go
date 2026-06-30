package forward

import (
	"sort"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

type Metrics struct {
	TCPConnections      int64
	TCPConnectionEvents int64
	UDPPackets          int64
	UploadBytes         int64
	DownloadBytes       int64
}

type targetMetricKey struct {
	ruleID   string
	targetID string
}

type targetMetricCounter struct {
	tcpConnections         int64
	tcpConnectionEvents    int64
	tcpDialReservations    int64
	udpSessions            int64
	udpSessionReservations int64
	udpPackets             int64
	uploadBytes            int64
	downloadBytes          int64
	latencyMS              int64
}

type targetMetricSnapshot struct {
	TCPConnections      int64
	TCPConnectionEvents int64
	UDPPackets          int64
	UploadBytes         int64
	DownloadBytes       int64
	LatencyMS           int64
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
		TCPConnections:      metrics.tcpConnections,
		TCPConnectionEvents: metrics.tcpConnectionEvents,
		UDPPackets:          metrics.udpPackets,
		UploadBytes:         metrics.uploadBytes,
		DownloadBytes:       metrics.downloadBytes,
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
			for _, target := range activeTargetGroupCandidates(rule) {
				if target.ID != "" {
					activeTargets[targetMetricKey{ruleID: rule.ID, targetID: target.ID}] = true
				}
			}
		}
	}
	return activeTargets
}

func (metrics *metricsCounter) openConnectionsForTargets(ruleID string, targets []agent.TargetEndpoint) map[string]int64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	counts := make(map[string]int64, len(targets))
	for _, target := range targets {
		if target.ID == "" {
			continue
		}
		counter := metrics.targets[targetMetricKey{ruleID: ruleID, targetID: target.ID}]
		if counter == nil {
			counts[target.ID] = 0
			continue
		}
		counts[target.ID] = counter.openConnections()
	}
	return counts
}

func (metrics *metricsCounter) pruneInactiveTargetsLocked() {
	if !metrics.activeApplied {
		return
	}
	for key, target := range metrics.targets {
		if metrics.activeTargets[key] {
			continue
		}
		if target.openConnections() <= 0 {
			metrics.queueFinalInactiveTargetDeltaLocked(key, target)
			delete(metrics.targets, key)
			delete(metrics.lastTargets, key)
			continue
		}
		target.latencyMS = 0
	}
}

func (metrics *metricsCounter) queueFinalInactiveTargetDeltaLocked(key targetMetricKey, target *targetMetricCounter) {
	if target == nil {
		return
	}
	current := targetMetricSnapshot{
		TCPConnections:      target.tcpConnections,
		TCPConnectionEvents: target.tcpConnectionEvents,
		UDPPackets:          target.udpPackets,
		UploadBytes:         target.uploadBytes,
		DownloadBytes:       target.downloadBytes,
		LatencyMS:           target.latencyMS,
	}
	if delta := targetTrafficDelta(key.ruleID, current, metrics.lastTargets[key]); delta != nil {
		metrics.pendingDeltas = append(metrics.pendingDeltas, *delta)
	}
}

func (metrics *metricsCounter) copyReportableTargetSnapshotsLocked() map[targetMetricKey]targetMetricSnapshot {
	snapshots := make(map[targetMetricKey]targetMetricSnapshot, len(metrics.targets))
	for key, target := range metrics.targets {
		if metrics.activeApplied && !metrics.activeTargets[key] && target.openConnections() <= 0 {
			continue
		}
		snapshots[key] = targetMetricSnapshot{
			TCPConnections:      target.tcpConnections,
			TCPConnectionEvents: target.tcpConnectionEvents,
			UDPPackets:          target.udpPackets,
			UploadBytes:         target.uploadBytes,
			DownloadBytes:       target.downloadBytes,
			LatencyMS:           target.latencyMS,
		}
	}
	return snapshots
}

func (metrics *metricsCounter) agentPayload(now time.Time) agent.MetricsPayload {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.pruneInactiveTargetsLocked()
	current := Metrics{
		TCPConnections:      metrics.tcpConnections,
		TCPConnectionEvents: metrics.tcpConnectionEvents,
		UDPPackets:          metrics.udpPackets,
		UploadBytes:         metrics.uploadBytes,
		DownloadBytes:       metrics.downloadBytes,
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
				if delta := targetTrafficDelta(key.ruleID, target, lastTarget); delta != nil {
					payload.TrafficDeltas = append(payload.TrafficDeltas, *delta)
				}
			}
		}
	}
	if len(metrics.pendingDeltas) > 0 {
		payload.TrafficDeltas = mergePayloadTrafficDeltas(append(metrics.pendingDeltas, payload.TrafficDeltas...))
		metrics.pendingDeltas = nil
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

func mergePayloadTrafficDeltas(deltas []agent.RuleTrafficDelta) []agent.RuleTrafficDelta {
	byRule := make(map[string]agent.RuleTrafficDelta)
	order := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		if delta.RuleID == "" {
			continue
		}
		current, ok := byRule[delta.RuleID]
		if !ok {
			order = append(order, delta.RuleID)
			current.RuleID = delta.RuleID
		}
		current.UploadBytes += delta.UploadBytes
		current.DownloadBytes += delta.DownloadBytes
		current.TCPConnections += delta.TCPConnections
		current.UDPPackets += delta.UDPPackets
		byRule[delta.RuleID] = current
	}
	merged := make([]agent.RuleTrafficDelta, 0, len(byRule))
	for _, ruleID := range order {
		delta := byRule[ruleID]
		if delta.UploadBytes == 0 && delta.DownloadBytes == 0 && delta.TCPConnections == 0 && delta.UDPPackets == 0 {
			continue
		}
		merged = append(merged, delta)
	}
	return merged
}

func targetTrafficDelta(ruleID string, current targetMetricSnapshot, last targetMetricSnapshot) *agent.RuleTrafficDelta {
	delta := agent.RuleTrafficDelta{
		RuleID:         ruleID,
		UploadBytes:    positiveDelta(current.UploadBytes, last.UploadBytes),
		DownloadBytes:  positiveDelta(current.DownloadBytes, last.DownloadBytes),
		TCPConnections: positiveDelta(current.TCPConnectionEvents, last.TCPConnectionEvents),
		UDPPackets:     positiveDelta(current.UDPPackets, last.UDPPackets),
	}
	if delta.UploadBytes == 0 && delta.DownloadBytes == 0 && delta.TCPConnections == 0 && delta.UDPPackets == 0 {
		return nil
	}
	return &delta
}

func positiveDelta(current int64, last int64) int64 {
	if current <= last {
		return 0
	}
	return current - last
}

func (metrics *metricsCounter) addTCPConnection(delta int64) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.tcpConnections += delta
	if delta > 0 {
		metrics.tcpConnectionEvents += delta
	}
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
	target := metrics.targets[key]
	if target == nil {
		if metrics.activeApplied && !metrics.activeTargets[key] {
			return nil, false
		}
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
	if delta > 0 {
		target.tcpConnectionEvents += delta
	}
	if target.tcpConnections < 0 {
		target.tcpConnections = 0
	}
	metrics.pruneClosedInactiveTargetLocked(key, target)
}

func (metrics *metricsCounter) reserveTargetTCPDial(ruleID string, targetID string) bool {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	target, ok := metrics.targetLocked(ruleID, targetID)
	if !ok {
		return false
	}
	target.tcpDialReservations++
	return true
}

func (metrics *metricsCounter) reserveLeastLoadTargetTCPDial(ruleID string, targets []agent.TargetEndpoint) (agent.TargetEndpoint, bool, bool) {
	if len(targets) == 0 {
		return agent.TargetEndpoint{}, false, false
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	best := targets[0]
	bestConnections := metrics.targetOpenConnectionsLocked(ruleID, best.ID)
	for _, candidate := range targets[1:] {
		candidateConnections := metrics.targetOpenConnectionsLocked(ruleID, candidate.ID)
		if candidateConnections*int64(best.Weight) < bestConnections*int64(candidate.Weight) {
			best = candidate
			bestConnections = candidateConnections
		}
	}
	target, ok := metrics.targetLocked(ruleID, best.ID)
	if !ok {
		return best, false, true
	}
	target.tcpDialReservations++
	return best, true, true
}

func (metrics *metricsCounter) reserveLeastLoadTargetUDPSession(ruleID string, targets []agent.TargetEndpoint) (agent.TargetEndpoint, bool, bool) {
	if len(targets) == 0 {
		return agent.TargetEndpoint{}, false, false
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	best := targets[0]
	bestConnections := metrics.targetOpenConnectionsLocked(ruleID, best.ID)
	for _, candidate := range targets[1:] {
		candidateConnections := metrics.targetOpenConnectionsLocked(ruleID, candidate.ID)
		if candidateConnections*int64(best.Weight) < bestConnections*int64(candidate.Weight) {
			best = candidate
			bestConnections = candidateConnections
		}
	}
	target, ok := metrics.targetLocked(ruleID, best.ID)
	if !ok {
		return best, false, true
	}
	target.udpSessionReservations++
	return best, true, true
}

func (metrics *metricsCounter) targetOpenConnectionsLocked(ruleID string, targetID string) int64 {
	return metrics.targets[targetMetricKey{ruleID: ruleID, targetID: targetID}].openConnections()
}

func (metrics *metricsCounter) releaseTargetTCPDial(ruleID string, targetID string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	target := metrics.targets[key]
	if target == nil {
		return
	}
	target.tcpDialReservations--
	if target.tcpDialReservations < 0 {
		target.tcpDialReservations = 0
	}
	metrics.pruneClosedInactiveTargetLocked(key, target)
}

func (metrics *metricsCounter) promoteTargetTCPDial(ruleID string, targetID string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	target := metrics.targets[key]
	if target == nil {
		target = &targetMetricCounter{}
		metrics.targets[key] = target
	}
	if target.tcpDialReservations > 0 {
		target.tcpDialReservations--
	}
	target.tcpConnections++
	target.tcpConnectionEvents++
}

func (metrics *metricsCounter) releaseTargetUDPSessionReservation(ruleID string, targetID string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	target := metrics.targets[key]
	if target == nil {
		return
	}
	target.udpSessionReservations--
	if target.udpSessionReservations < 0 {
		target.udpSessionReservations = 0
	}
	metrics.pruneClosedInactiveTargetLocked(key, target)
}

func (metrics *metricsCounter) promoteTargetUDPSessionReservation(ruleID string, targetID string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	key := targetMetricKey{ruleID: ruleID, targetID: targetID}
	target := metrics.targets[key]
	if target == nil {
		target = &targetMetricCounter{}
		metrics.targets[key] = target
	}
	if target.udpSessionReservations > 0 {
		target.udpSessionReservations--
	}
	target.udpSessions++
}

func (metrics *metricsCounter) addTargetUDPSession(ruleID string, targetID string, delta int64) {
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
	target.udpSessions += delta
	if target.udpSessions < 0 {
		target.udpSessions = 0
	}
	metrics.pruneClosedInactiveTargetLocked(key, target)
}

func (metrics *metricsCounter) pruneClosedInactiveTargetLocked(key targetMetricKey, target *targetMetricCounter) {
	if metrics.activeApplied && !metrics.activeTargets[key] && target.openConnections() <= 0 {
		metrics.queueFinalInactiveTargetDeltaLocked(key, target)
		delete(metrics.targets, key)
		delete(metrics.lastTargets, key)
	}
}

func (target *targetMetricCounter) openConnections() int64 {
	if target == nil {
		return 0
	}
	return target.tcpConnections + target.tcpDialReservations + target.udpSessions + target.udpSessionReservations
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
