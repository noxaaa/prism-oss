package service

import (
	"log"
	"net"
	"strings"
	"sync"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/oschwald/maxminddb-golang"
)

const (
	defaultGeoIPDBPath     = "/data/geoip/dbip-country-lite.mmdb"
	dbIPCountryAttribution = "IP geolocation by DB-IP (db-ip.com), CC BY 4.0"
)

type GeoIPResolver interface {
	Lookup(ip net.IP) GeoIPResult
}

type GeoIPResult struct {
	CountryCode string
	CountryName string
	Attribution string
}

type mmdbGeoIPResolver struct {
	path   string
	mu     sync.Mutex
	reader *maxminddb.Reader
	logged bool
}

func NewGeoIPResolver(path string) GeoIPResolver {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultGeoIPDBPath
	}
	return &mmdbGeoIPResolver{path: path}
}

func (resolver *mmdbGeoIPResolver) Lookup(ip net.IP) GeoIPResult {
	if !publicGeoIPLookupIP(ip) {
		return GeoIPResult{}
	}
	reader, err := resolver.readerOrError()
	if err != nil {
		resolver.logLoadError(err)
		return GeoIPResult{Attribution: dbIPCountryAttribution}
	}
	var record struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
	}
	if err := reader.Lookup(ip, &record); err != nil {
		return GeoIPResult{Attribution: dbIPCountryAttribution}
	}
	code := strings.ToUpper(strings.TrimSpace(record.Country.ISOCode))
	if code == "" {
		return GeoIPResult{Attribution: dbIPCountryAttribution}
	}
	name := strings.TrimSpace(record.Country.Names["en"])
	if name == "" {
		name = strings.TrimSpace(record.Country.Names["zh-CN"])
	}
	return GeoIPResult{CountryCode: code, CountryName: name, Attribution: dbIPCountryAttribution}
}

func (resolver *mmdbGeoIPResolver) readerOrError() (*maxminddb.Reader, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.reader != nil {
		return resolver.reader, nil
	}
	reader, err := maxminddb.Open(resolver.path)
	if err != nil {
		return nil, err
	}
	resolver.reader = reader
	return resolver.reader, nil
}

func (resolver *mmdbGeoIPResolver) logLoadError(err error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.logged {
		return
	}
	resolver.logged = true
	log.Printf("geoip database unavailable at %s: %v", resolver.path, err)
}

func publicGeoIPLookupIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() && !isCarrierGradeNATIP(ip)
}

func (service *ControlService) toNodePayload(node repo.NodeRecord) NodePayload {
	return nodePayloadWithGeoIP(node, service.geoIPResolver)
}

func nodePayloadWithGeoIP(node repo.NodeRecord, resolver GeoIPResolver) NodePayload {
	payload := nodePayloadWithoutGeoIP(node)
	payload.DNSPublishAddresses = toNodeDNSPublishAddressPayloadsWithGeoIP(node.DNSPublishAddresses, resolver)
	payload.GeoIP = selectNodeGeoIP(payload.DNSPublishAddresses)
	return payload
}

func selectNodeGeoIP(addresses []NodeDNSPublishAddressPayload) NodeGeoIPPayload {
	for _, source := range []string{"MANUAL", "AUTO"} {
		for _, address := range addresses {
			if address.Enabled && strings.EqualFold(address.Source, source) && address.GeoIP.IP != "" {
				return address.GeoIP
			}
		}
	}
	return NodeGeoIPPayload{Attribution: dbIPCountryAttribution}
}

func toNodeDNSPublishAddressPayloadsWithGeoIP(addresses []repo.NodeDNSPublishAddressRecord, resolver GeoIPResolver) []NodeDNSPublishAddressPayload {
	payloads := make([]NodeDNSPublishAddressPayload, 0, len(addresses))
	for _, address := range addresses {
		payload := toNodeDNSPublishAddressPayload(address)
		if address.Enabled {
			payload.GeoIP = geoIPPayloadForAddress(address.Address, address.Source, resolver)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func geoIPPayloadForAddress(rawIP string, source string, resolver GeoIPResolver) NodeGeoIPPayload {
	ip := net.ParseIP(strings.TrimSpace(rawIP))
	if !publicGeoIPLookupIP(ip) {
		return NodeGeoIPPayload{Attribution: dbIPCountryAttribution}
	}
	payload := NodeGeoIPPayload{IP: ip.String(), Source: strings.ToUpper(strings.TrimSpace(source)), Attribution: dbIPCountryAttribution}
	if resolver == nil {
		return payload
	}
	result := resolver.Lookup(ip)
	payload.CountryCode = strings.ToUpper(strings.TrimSpace(result.CountryCode))
	payload.CountryName = strings.TrimSpace(result.CountryName)
	payload.FlagEmoji = countryFlagEmoji(payload.CountryCode)
	if strings.TrimSpace(result.Attribution) != "" {
		payload.Attribution = strings.TrimSpace(result.Attribution)
	}
	return payload
}

func countryFlagEmoji(countryCode string) string {
	code := strings.ToUpper(strings.TrimSpace(countryCode))
	if len(code) != 2 {
		return ""
	}
	first := rune(code[0])
	second := rune(code[1])
	if first < 'A' || first > 'Z' || second < 'A' || second > 'Z' {
		return ""
	}
	return string([]rune{0x1F1E6 + first - 'A', 0x1F1E6 + second - 'A'})
}
