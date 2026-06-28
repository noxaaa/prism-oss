package repo

import "strings"

const (
	DataplaneModeAuto     = "AUTO"
	DataplaneModeNative   = "NATIVE"
	DataplaneModeHAProxy  = "HAPROXY"
	DataplaneModeNFTables = "NFTABLES"

	DataplaneConflictPolicyFailFast = "FAIL_FAST"
)

func normalizeNodeDataplaneMode(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DataplaneModeNative, DataplaneModeHAProxy, DataplaneModeNFTables:
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return DataplaneModeAuto
	}
}

func normalizeNodeDataplaneConflictPolicy(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DataplaneConflictPolicyFailFast:
		return DataplaneConflictPolicyFailFast
	default:
		return DataplaneConflictPolicyFailFast
	}
}

func normalizeRuleDataplanePreference(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DataplaneModeNative, DataplaneModeHAProxy, DataplaneModeNFTables:
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return DataplaneModeAuto
	}
}
