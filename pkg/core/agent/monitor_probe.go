package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var runMonitorProbeCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (runtime *NodeRuntime) setMonitorSnapshot(snapshot MonitorConfigSnapshot) {
	runtime.monitorMu.Lock()
	defer runtime.monitorMu.Unlock()
	runtime.monitorSnapshot = snapshot
	if runtime.monitorLastProbe == nil {
		runtime.monitorLastProbe = map[string]time.Time{}
	}
}

func (runtime *NodeRuntime) collectDueHealthResults(ctx context.Context) []HealthResultPayload {
	runtime.monitorMu.Lock()
	defer runtime.monitorMu.Unlock()
	if runtime.monitorLastProbe == nil {
		runtime.monitorLastProbe = map[string]time.Time{}
	}
	now := time.Now().UTC()
	results := make([]HealthResultPayload, 0)
	for _, check := range runtime.monitorSnapshot.HealthChecks {
		interval := time.Duration(check.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 30 * time.Second
		}
		for _, target := range check.Targets {
			key := check.ID + "\x00" + target.HealthCheckTargetID
			if lastProbe := runtime.monitorLastProbe[key]; !lastProbe.IsZero() && now.Sub(lastProbe) < interval {
				continue
			}
			runtime.monitorLastProbe[key] = now
			results = append(results, runtime.probeHealthTarget(ctx, check, target, now))
		}
	}
	return results
}

func (runtime *NodeRuntime) probeHealthTarget(ctx context.Context, check MonitorHealthCheck, target MonitorHealthTarget, observedAt time.Time) HealthResultPayload {
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	startedAt := time.Now()
	status, err := runHealthProbe(probeCtx, check, target, timeout)
	latency := int(time.Since(startedAt).Milliseconds())
	errorMessage := ""
	if err != nil {
		status = "OFFLINE"
		errorMessage = err.Error()
	}
	return HealthResultPayload{
		HealthCheckID:       check.ID,
		HealthCheckTargetID: target.HealthCheckTargetID,
		TargetID:            target.TargetID,
		Status:              status,
		LatencyMS:           latency,
		ErrorMessage:        errorMessage,
		ObservedAt:          observedAt.Format(time.RFC3339Nano),
	}
}

func runHealthProbe(ctx context.Context, check MonitorHealthCheck, target MonitorHealthTarget, timeout time.Duration) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(check.ProbeType)) {
	case "TCP_PORT":
		return runTCPHealthProbe(ctx, target.Host, effectiveProbePort(check, target.Port))
	case "HTTP":
		return runHTTPHealthProbe(ctx, check, target)
	case "ICMP":
		return runICMPHealthProbe(ctx, target.Host, timeout)
	default:
		return "UNKNOWN", fmt.Errorf("unsupported probe type %s", check.ProbeType)
	}
}

func runTCPHealthProbe(ctx context.Context, host string, port int) (string, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return "OFFLINE", err
	}
	_ = conn.Close()
	return "ONLINE", nil
}

func runHTTPHealthProbe(ctx context.Context, check MonitorHealthCheck, target MonitorHealthTarget) (string, error) {
	config := map[string]any{}
	_ = json.Unmarshal([]byte(check.ConfigJSON), &config)
	scheme := stringConfig(config, "scheme", "http")
	path := stringConfig(config, "path", "/")
	method := stringConfig(config, "method", http.MethodGet)
	port := effectiveProbePort(check, target.Port)
	url := fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(target.Host, strconv.Itoa(port)), path)
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return "OFFLINE", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "OFFLINE", err
	}
	defer func() { _ = response.Body.Close() }()
	if !expectedHTTPStatus(config, response.StatusCode) {
		return "OFFLINE", fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	return "ONLINE", nil
}

func runICMPHealthProbe(ctx context.Context, host string, timeout time.Duration) (string, error) {
	if output, err := runMonitorProbeCommand(ctx, "ping", "-c", "1", "-W", strconv.Itoa(pingTimeoutSeconds(timeout)), host); err != nil {
		return "OFFLINE", fmt.Errorf("ping failed: %s", strings.TrimSpace(string(output)))
	}
	return "ONLINE", nil
}

func pingTimeoutSeconds(timeout time.Duration) int {
	if timeout <= 0 {
		return 1
	}
	seconds := int(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func effectiveProbePort(check MonitorHealthCheck, fallback int) int {
	config := map[string]any{}
	_ = json.Unmarshal([]byte(check.ConfigJSON), &config)
	if value, ok := config["port_override"].(float64); ok && value >= 1 && value <= 65535 {
		return int(value)
	}
	return fallback
}

func stringConfig(config map[string]any, key string, fallback string) string {
	if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func expectedHTTPStatus(config map[string]any, status int) bool {
	raw, ok := config["expected_statuses"].([]any)
	if !ok || len(raw) == 0 {
		return status >= 200 && status < 400
	}
	for _, value := range raw {
		if numeric, ok := value.(float64); ok && int(numeric) == status {
			return true
		}
	}
	return false
}
