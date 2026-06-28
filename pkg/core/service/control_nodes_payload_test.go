package service

import (
	"encoding/json"
	"net"
	"os"
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

func TestNormalizeNodeDataplaneModeForMutationRejectsInvalidNonEmptyValue(t *testing.T) {
	if _, err := normalizeNodeDataplaneModeForMutation("HAPR0XY"); err == nil {
		t.Fatalf("expected invalid service node dataplane mode to be rejected")
	}
	value, err := normalizeNodeDataplaneModeForMutation("")
	if err != nil {
		t.Fatalf("normalize blank node dataplane mode: %v", err)
	}
	if value != NodeDataplaneModeAuto {
		t.Fatalf("blank node dataplane mode = %q, want AUTO", value)
	}
}

func TestUpdateNodeCoalescesDesiredConfigVersionIncrement(t *testing.T) {
	source, err := os.ReadFile("control_nodes.go")
	if err != nil {
		t.Fatalf("read control_nodes.go: %v", err)
	}
	count := strings.Count(string(source), "IncrementDesiredConfigForNode(ctx, identity.OrganizationID, node.ID")
	if count != 1 {
		t.Fatalf("UpdateNode must coalesce dataplane and membership config bumps into one increment, found %d", count)
	}
}

type staticGeoIPResolver map[string]GeoIPResult

func (resolver staticGeoIPResolver) Lookup(ip net.IP) GeoIPResult {
	return resolver[ip.String()]
}
