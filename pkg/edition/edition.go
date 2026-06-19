package edition

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Key string

const (
	KeyOSS Key = "oss"
)

var ErrUnsupportedEdition = errors.New("unsupported edition")

var (
	providersMu sync.RWMutex
	providers   = map[Key]Provider{}
)

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

func RegisterProvider(provider Provider) error {
	if provider == nil {
		return errors.New("edition provider is required")
	}
	key := provider.Key()
	if key == "" {
		return errors.New("edition provider key is required")
	}

	providersMu.Lock()
	defer providersMu.Unlock()
	if _, exists := providers[key]; exists {
		return fmt.Errorf("edition provider %q is already registered", key)
	}
	providers[key] = provider
	return nil
}

func MustRegisterProvider(provider Provider) {
	if err := RegisterProvider(provider); err != nil {
		panic(err)
	}
}

func ProviderForKey(key Key) (Provider, error) {
	if key == "" {
		key = defaultKey()
	}

	providersMu.RLock()
	provider, ok := providers[key]
	providersMu.RUnlock()
	if ok {
		return provider, nil
	}
	return nil, fmt.Errorf("%w %q", ErrUnsupportedEdition, key)
}

func KeyFromString(value string) (Key, error) {
	key := Key(strings.TrimSpace(value))
	if key == "" {
		return defaultKey(), nil
	}
	if _, err := ProviderForKey(key); err != nil {
		return "", err
	}
	return key, nil
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
