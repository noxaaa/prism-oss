package service

import "strings"

const (
	NodeDataplaneModeAuto     = "AUTO"
	NodeDataplaneModeNative   = "NATIVE"
	NodeDataplaneModeHAProxy  = "HAPROXY"
	NodeDataplaneModeNFTables = "NFTABLES"

	NodeDataplaneConflictPolicyFailFast = "FAIL_FAST"

	NodeDataplaneStatusUnknown = "UNKNOWN"
	NodeDataplaneStatusHealthy = "HEALTHY"
	NodeDataplaneStatusFailed  = "FAILED"
	NodeDataplaneStatusDrifted = "DRIFTED"
)

func defaultNodeDataplaneMode(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case NodeDataplaneModeNative, NodeDataplaneModeHAProxy, NodeDataplaneModeNFTables:
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return NodeDataplaneModeAuto
	}
}

func normalizeNodeDataplaneModeForMutation(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if err := validateNodeDataplaneMode(value); err != nil {
		return "", err
	}
	if value == "" {
		return NodeDataplaneModeAuto, nil
	}
	return value, nil
}

func defaultNodeDataplaneConflictPolicy(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case NodeDataplaneConflictPolicyFailFast:
		return NodeDataplaneConflictPolicyFailFast
	default:
		return NodeDataplaneConflictPolicyFailFast
	}
}

func defaultNodeDataplaneStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case NodeDataplaneStatusHealthy, NodeDataplaneStatusFailed, NodeDataplaneStatusDrifted:
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return NodeDataplaneStatusUnknown
	}
}

func validateNodeDataplaneMode(value string) error {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", NodeDataplaneModeAuto, NodeDataplaneModeNative, NodeDataplaneModeHAProxy, NodeDataplaneModeNFTables:
		return nil
	default:
		return validationFieldError("dataplane_mode", "Unsupported node dataplane mode.", map[string]any{"actual": value})
	}
}

func validateNodeDataplaneConflictPolicy(value string) error {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", NodeDataplaneConflictPolicyFailFast:
		return nil
	default:
		return validationFieldError("dataplane_conflict_policy", "Unsupported node dataplane conflict policy.", map[string]any{"actual": value})
	}
}
