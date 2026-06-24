package agent

import (
	"errors"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type ConfigSnapshot struct {
	AgentProtocolVersion ProtocolVersion `json:"agent_protocol_version"`
	NodeID               string          `json:"node_id"`
	ConfigVersion        int             `json:"config_version"`
	ConfigHash           string          `json:"config_hash"`
	Rules                []RuleConfig    `json:"rules"`
}

type MonitorConfigSnapshot struct {
	MonitorID      string               `json:"monitor_id"`
	ConfigVersion  int                  `json:"config_version"`
	GeneratedAtUTC string               `json:"generated_at"`
	HealthChecks   []MonitorHealthCheck `json:"health_checks"`
}

type MonitorHealthCheck struct {
	ID              string                `json:"id"`
	ProbeType       string                `json:"probe_type"`
	IntervalSeconds int                   `json:"interval_seconds"`
	TimeoutSeconds  int                   `json:"timeout_seconds"`
	ConfigJSON      string                `json:"config_json"`
	Targets         []MonitorHealthTarget `json:"targets"`
}

type MonitorHealthTarget struct {
	HealthCheckTargetID string `json:"health_check_target_id"`
	TargetID            string `json:"target_id"`
	Name                string `json:"name"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
}

type HealthResultPayload struct {
	HealthCheckID       string `json:"health_check_id"`
	HealthCheckTargetID string `json:"health_check_target_id"`
	TargetID            string `json:"target_id"`
	Status              string `json:"status"`
	LatencyMS           int    `json:"latency_ms,omitempty"`
	ErrorMessage        string `json:"error_message,omitempty"`
	ObservedAt          string `json:"observed_at"`
}

type RuleConfig struct {
	ID               string                `json:"id"`
	ConfigVersion    int                   `json:"config_version"`
	Enabled          bool                  `json:"enabled"`
	ForwardingType   domain.ForwardingType `json:"forwarding_type"`
	Protocol         domain.Protocol       `json:"protocol"`
	NodeIDs          []string              `json:"node_ids,omitempty"`
	NodeGroupIDs     []string              `json:"node_group_ids,omitempty"`
	ListenIP         string                `json:"listen_ip"`
	Port             int                   `json:"port"`
	MatchType        string                `json:"match_type"`
	SNIHostname      string                `json:"sni_hostname,omitempty"`
	ProxyProtocolIn  string                `json:"proxy_protocol_in"`
	ProxyProtocolOut string                `json:"proxy_protocol_out"`
	Upstream         RuleUpstreamConfig    `json:"upstream"`
}

type RuleUpstreamConfig struct {
	Type        string                 `json:"type"`
	Target      *TargetEndpoint        `json:"target,omitempty"`
	TargetGroup []TargetPriorityBucket `json:"target_group,omitempty"`
}

type TargetPriorityBucket struct {
	Priority int              `json:"priority"`
	Targets  []TargetEndpoint `json:"targets"`
}

type TargetEndpoint struct {
	ID      string `json:"id"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

type MetricsPayload struct {
	TCPConnections       int64                  `json:"tcp_connections"`
	UDPPacketsPerSecond  int64                  `json:"udp_packets_per_second"`
	BandwidthBps         int64                  `json:"bandwidth_bps"`
	CPUPercent           float64                `json:"cpu_percent"`
	CPUModel             string                 `json:"cpu_model"`
	CPULogicalCores      int                    `json:"cpu_logical_cores"`
	CPUPhysicalCores     int                    `json:"cpu_physical_cores"`
	RAMUsedBytes         uint64                 `json:"ram_used_bytes"`
	RAMTotalBytes        uint64                 `json:"ram_total_bytes"`
	UploadBytes          int64                  `json:"upload_bytes"`
	DownloadBytes        int64                  `json:"download_bytes"`
	UptimeSeconds        int64                  `json:"uptime_seconds"`
	BootTime             string                 `json:"boot_time"`
	OSName               string                 `json:"os_name"`
	OSVersion            string                 `json:"os_version"`
	KernelVersion        string                 `json:"kernel_version"`
	Architecture         string                 `json:"architecture"`
	VirtualizationSystem string                 `json:"virtualization_system"`
	VirtualizationRole   string                 `json:"virtualization_role"`
	AppliedConfigVersion int                    `json:"applied_config_version"`
	Targets              []TargetMetricsPayload `json:"targets"`
	TrafficReportID      string                 `json:"traffic_report_id,omitempty"`
	TrafficDeltas        []RuleTrafficDelta     `json:"traffic_deltas,omitempty"`
}

type TargetMetricsPayload struct {
	RuleID              string `json:"rule_id"`
	TargetID            string `json:"target_id"`
	TCPConnections      int64  `json:"tcp_connections"`
	UDPPacketsPerSecond int64  `json:"udp_packets_per_second"`
	BandwidthBps        int64  `json:"bandwidth_bps"`
	UploadBytes         int64  `json:"upload_bytes"`
	DownloadBytes       int64  `json:"download_bytes"`
	LatencyMS           int64  `json:"latency_ms"`
}

type RuleTrafficDelta struct {
	RuleID         string `json:"rule_id"`
	UploadBytes    int64  `json:"upload_bytes"`
	DownloadBytes  int64  `json:"download_bytes"`
	TCPConnections int64  `json:"tcp_connections"`
	UDPPackets     int64  `json:"udp_packets"`
}

type ConfigApplyErrorDetail struct {
	Code     string          `json:"code"`
	RuleIDs  []string        `json:"rule_ids"`
	Protocol domain.Protocol `json:"protocol"`
	ListenIP string          `json:"listen_ip"`
	Port     int             `json:"port"`
	Message  string          `json:"message"`
}

type ConfigApplyError struct {
	Message string
	Errors  []ConfigApplyErrorDetail
}

func (err ConfigApplyError) Error() string {
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	if len(err.Errors) > 0 {
		return err.Errors[0].Message
	}
	return "config apply failed"
}

func StructuredApplyErrors(err error) []ConfigApplyErrorDetail {
	if err == nil {
		return nil
	}
	var applyErr ConfigApplyError
	if !errors.As(err, &applyErr) {
		return nil
	}
	return append([]ConfigApplyErrorDetail(nil), applyErr.Errors...)
}
