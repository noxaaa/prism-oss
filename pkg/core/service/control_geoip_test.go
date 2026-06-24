package service

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func TestGeoIPResolverReadsCountryMMDB(t *testing.T) {
	dbPath := writeTestCountryMMDB(t)
	resolver := NewGeoIPResolver(dbPath)

	result := resolver.Lookup(net.ParseIP("8.8.8.8"))
	if result.CountryCode != "US" {
		t.Fatalf("country code = %q, want US", result.CountryCode)
	}
	if result.CountryName != "United States" {
		t.Fatalf("country name = %q, want United States", result.CountryName)
	}
	if result.Attribution == "" {
		t.Fatalf("expected GeoIP attribution")
	}
}

func TestGeoIPResolverReturnsUnknownForMissingDatabaseAndPrivateIP(t *testing.T) {
	resolver := NewGeoIPResolver(filepath.Join(t.TempDir(), "missing.mmdb"))
	if result := resolver.Lookup(net.ParseIP("10.0.0.1")); result.CountryCode != "" || result.CountryName != "" {
		t.Fatalf("private IP must not be looked up, got %#v", result)
	}
	if result := resolver.Lookup(net.ParseIP("8.8.8.8")); result.CountryCode != "" || result.CountryName != "" {
		t.Fatalf("missing database must return unknown country, got %#v", result)
	}
}

func TestGeoIPResolverRetriesAfterMissingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "country.mmdb")
	resolver := NewGeoIPResolver(dbPath)
	if result := resolver.Lookup(net.ParseIP("8.8.8.8")); result.CountryCode != "" || result.CountryName != "" {
		t.Fatalf("missing database must return unknown country, got %#v", result)
	}
	writeTestCountryMMDBAt(t, dbPath)
	result := resolver.Lookup(net.ParseIP("8.8.8.8"))
	if result.CountryCode != "US" {
		t.Fatalf("country code after database appears = %q, want US", result.CountryCode)
	}
}

func writeTestCountryMMDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "country.mmdb")
	writeTestCountryMMDBAt(t, path)
	return path
}

func writeTestCountryMMDBAt(t *testing.T, path string) {
	t.Helper()
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "DBIP-Country-Lite",
		Languages:    []string{"en", "zh-CN"},
		IPVersion:    4,
	})
	if err != nil {
		t.Fatalf("create mmdb writer: %v", err)
	}
	_, network, err := net.ParseCIDR("8.8.8.0/24")
	if err != nil {
		t.Fatalf("parse test cidr: %v", err)
	}
	record := mmdbtype.Map{
		"country": mmdbtype.Map{
			"iso_code": mmdbtype.String("US"),
			"names": mmdbtype.Map{
				"en":    mmdbtype.String("United States"),
				"zh-CN": mmdbtype.String("美国"),
			},
		},
	}
	if err := tree.Insert(network, record); err != nil {
		t.Fatalf("insert mmdb record: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create mmdb: %v", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := tree.WriteTo(file); err != nil {
		t.Fatalf("write mmdb: %v", err)
	}
}
