package service

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func composeDNSManagedRecordName(recordHost string, recordName string, zoneName string) (string, string, error) {
	zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(zoneName), "."))
	recordHost = strings.ToLower(strings.Trim(strings.TrimSpace(recordHost), "."))
	recordName = strings.ToLower(strings.Trim(strings.TrimSpace(recordName), "."))
	if recordHost == "" && recordName != "" {
		if recordName == zoneName {
			return "@", zoneName, nil
		}
		suffix := "." + zoneName
		if !strings.HasSuffix(recordName, suffix) {
			return "", "", ErrInvalidInput
		}
		recordHost = strings.TrimSuffix(recordName, suffix)
	}
	if recordHost == "" || recordHost == "@" {
		return "@", zoneName, nil
	}
	if recordHost == zoneName || strings.HasSuffix(recordHost, "."+zoneName) {
		return "", "", ErrInvalidInput
	}
	if !validDNSRelativeHost(recordHost) {
		return "", "", ErrInvalidInput
	}
	return recordHost, recordHost + "." + zoneName, nil
}

func validDNSRelativeHost(recordHost string) bool {
	labels := strings.Split(recordHost, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if character >= 'a' && character <= 'z' {
				continue
			}
			if character >= '0' && character <= '9' {
				continue
			}
			if character == '-' {
				continue
			}
			if character == '_' {
				continue
			}
			return false
		}
	}
	return true
}

func nodeMatchesGroupSet(node repo.NodeRecord, groupSet map[string]bool) bool {
	if len(groupSet) == 0 {
		return true
	}
	for _, groupID := range node.GroupIDs {
		if groupSet[groupID] {
			return true
		}
	}
	return false
}

func limitDNSAnswers(values []string, answerCount int) []string {
	if answerCount == -1 || answerCount >= len(values) {
		return values
	}
	if answerCount <= 0 {
		return nil
	}
	return values[:answerCount]
}

func normalizeDNSValues(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Trim(strings.TrimSpace(value), ".")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func dnsValuesMatchRecordType(values []string, recordType string) bool {
	for _, value := range values {
		address, err := netip.ParseAddr(value)
		if err != nil {
			return false
		}
		switch recordType {
		case "A":
			if !address.Is4() {
				return false
			}
		case "AAAA":
			if !address.Is6() || address.Is4() {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func normalizeDNSActionType(actionType string) string {
	return strings.ToUpper(strings.TrimSpace(actionType))
}
