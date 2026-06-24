package service

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestNodeAndMonitorPayloadGroupIDsEncodeAsEmptyArrays(t *testing.T) {
	payloads := []any{
		toNodePayload(repo.NodeRecord{}),
		toNodePayload(repo.NodeRecord{GroupIDs: []string{}}),
		toMonitorPayload(repo.MonitorRecord{}),
		toMonitorPayload(repo.MonitorRecord{GroupIDs: []string{}}),
	}
	for _, payload := range payloads {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		text := string(data)
		if strings.Contains(text, `"group_ids":null`) {
			t.Fatalf("group_ids must encode as an empty array, got %s", text)
		}
		if !strings.Contains(text, `"group_ids":[]`) {
			t.Fatalf("expected group_ids empty array in payload, got %s", text)
		}
	}
}

func TestNodePayloadGeoIPPrefersEnabledManualPublishAddress(t *testing.T) {
	service := NewControlServiceWithOptions(nil, ControlServiceOptions{GeoIPResolver: staticGeoIPResolver{
		"8.8.8.8": GeoIPResult{CountryCode: "US", CountryName: "United States"},
		"1.1.1.1": GeoIPResult{CountryCode: "AU", CountryName: "Australia"},
	}})
	payload := service.toNodePayload(repo.NodeRecord{
		DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{
			{ID: "disabled_manual", AddressType: "A", Address: "9.9.9.9", Source: "MANUAL", Enabled: false},
			{ID: "auto", AddressType: "A", Address: "1.1.1.1", Source: "AUTO", Enabled: true},
			{ID: "manual", AddressType: "A", Address: "8.8.8.8", Source: "MANUAL", Enabled: true},
		},
	})
	if payload.GeoIP.IP != "8.8.8.8" || payload.GeoIP.Source != "MANUAL" {
		t.Fatalf("expected manual publish address GeoIP, got %#v", payload.GeoIP)
	}
	if payload.GeoIP.CountryCode != "US" || payload.GeoIP.FlagEmoji != "🇺🇸" {
		t.Fatalf("expected US GeoIP flag, got %#v", payload.GeoIP)
	}
}

func TestNodePayloadGeoIPFallsBackToAutoPublishAddress(t *testing.T) {
	service := NewControlServiceWithOptions(nil, ControlServiceOptions{GeoIPResolver: staticGeoIPResolver{
		"1.1.1.1": GeoIPResult{CountryCode: "AU", CountryName: "Australia"},
	}})
	payload := service.toNodePayload(repo.NodeRecord{
		DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{
			{ID: "private_manual", AddressType: "A", Address: "10.0.0.1", Source: "MANUAL", Enabled: true},
			{ID: "auto", AddressType: "A", Address: "1.1.1.1", Source: "AUTO", Enabled: true},
		},
	})
	if payload.GeoIP.IP != "1.1.1.1" || payload.GeoIP.Source != "AUTO" {
		t.Fatalf("expected auto publish address GeoIP fallback, got %#v", payload.GeoIP)
	}
	if payload.GeoIP.CountryCode != "AU" || payload.GeoIP.FlagEmoji != "🇦🇺" {
		t.Fatalf("expected AU GeoIP flag, got %#v", payload.GeoIP)
	}
}

type staticGeoIPResolver map[string]GeoIPResult

func (resolver staticGeoIPResolver) Lookup(ip net.IP) GeoIPResult {
	return resolver[ip.String()]
}
