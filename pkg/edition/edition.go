package edition

import (
	"errors"
	"fmt"
	"strings"
)

type Key string

const (
	KeyOSS  Key = "oss"
	KeyFull Key = "full"
)

var ErrUnsupportedEdition = errors.New("unsupported edition")

type Capability string

const (
	CapabilityCoreForwarding   Capability = "core_forwarding"
	CapabilityTargets          Capability = "targets"
	CapabilityRules            Capability = "rules"
	CapabilityNodes            Capability = "nodes"
	CapabilityMonitors         Capability = "monitors"
	CapabilityBasicMetrics     Capability = "basic_metrics"
	CapabilitySingleUserAuth   Capability = "single_user_auth"
	CapabilityRBAC             Capability = "rbac"
	CapabilityMultiUser        Capability = "multi_user"
	CapabilityCommercialHealth Capability = "commercial_health"
	CapabilityDNS              Capability = "dns"
)

type Provider interface {
	Key() Key
	Capabilities() []Capability
	Has(Capability) bool
	DefaultMigrationDirs() []string
}

type staticProvider struct {
	key           Key
	capabilities  []Capability
	migrationDirs []string
}

func OSSProvider() Provider {
	return staticProvider{
		key: KeyOSS,
		capabilities: []Capability{
			CapabilityCoreForwarding,
			CapabilityTargets,
			CapabilityRules,
			CapabilityNodes,
			CapabilityBasicMetrics,
			CapabilitySingleUserAuth,
		},
		migrationDirs: []string{"migrations/core"},
	}
}

func FullProvider() Provider {
	return staticProvider{
		key: KeyFull,
		capabilities: []Capability{
			CapabilityCoreForwarding,
			CapabilityTargets,
			CapabilityRules,
			CapabilityNodes,
			CapabilityMonitors,
			CapabilityBasicMetrics,
			CapabilitySingleUserAuth,
			CapabilityRBAC,
			CapabilityMultiUser,
			CapabilityCommercialHealth,
			CapabilityDNS,
		},
		migrationDirs: []string{"migrations/core", "migrations/commercial"},
	}
}

func ProviderForKey(key Key) (Provider, error) {
	switch key {
	case KeyOSS:
		return OSSProvider(), nil
	case KeyFull:
		return FullProvider(), nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedEdition, key)
	}
}

func KeyFromString(value string) (Key, error) {
	switch strings.TrimSpace(value) {
	case "":
		return defaultKey(), nil
	case string(KeyOSS):
		return KeyOSS, nil
	case string(KeyFull):
		return KeyFull, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedEdition, value)
	}
}

func (provider staticProvider) Key() Key {
	return provider.key
}

func (provider staticProvider) Capabilities() []Capability {
	return append([]Capability(nil), provider.capabilities...)
}

func (provider staticProvider) Has(capability Capability) bool {
	for _, candidate := range provider.capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

func (provider staticProvider) DefaultMigrationDirs() []string {
	return append([]string(nil), provider.migrationDirs...)
}
