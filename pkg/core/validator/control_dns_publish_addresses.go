package validator

import (
	"net"
	"strings"
)

func validateDNSPublishAddresses(values []NodeDNSPublishAddress) ([]NodeDNSPublishAddress, error) {
	normalized := make([]NodeDNSPublishAddress, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value.AddressType = strings.ToUpper(strings.TrimSpace(value.AddressType))
		value.Address = strings.TrimSpace(value.Address)
		if value.Address == "" {
			continue
		}
		ip := net.ParseIP(value.Address)
		if ip == nil || !isPublicIP(ip) {
			return nil, ErrInvalidRequest
		}
		if value.AddressType == "" {
			if ip.To4() == nil {
				value.AddressType = "AAAA"
			} else {
				value.AddressType = "A"
			}
		}
		if (value.AddressType == "A" && ip.To4() == nil) || (value.AddressType == "AAAA" && ip.To4() != nil) {
			return nil, ErrInvalidRequest
		}
		if value.AddressType != "A" && value.AddressType != "AAAA" {
			return nil, ErrInvalidRequest
		}
		key := value.AddressType + "\x00" + value.Address
		if seen[key] {
			return nil, ErrInvalidRequest
		}
		seen[key] = true
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() && !isCarrierGradeNATAddress(ip)
}

func isCarrierGradeNATAddress(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 0x40
}
