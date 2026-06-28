package dataplane

import (
	"context"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

const (
	ModeAuto     = "AUTO"
	ModeNative   = "NATIVE"
	ModeHAProxy  = "HAPROXY"
	ModeNFTables = "NFTABLES"

	ConflictPolicyFailFast = "FAIL_FAST"

	ErrorListenerConflict                  = "LISTENER_CONFLICT"
	ErrorListenerOwnedByOtherPrismInstance = "LISTENER_OWNED_BY_OTHER_PRISM_INSTANCE"
	ErrorListenerOwnedByExternalProcess    = "LISTENER_OWNED_BY_EXTERNAL_PROCESS"
	ErrorDataplaneUnsupportedRule          = "DATAPLANE_UNSUPPORTED_RULE"
	ErrorDataplaneDrift                    = "DATAPLANE_DRIFT"
	ErrorNFTablesLocked                    = "NFTABLES_LOCKED"
	ErrorHAProxyValidateFailed             = "HAPROXY_VALIDATE_FAILED"
)

type Backend interface {
	Name() string
	Capabilities() Capabilities
	Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error
	Status(ctx context.Context) Status
	CleanupOwnedState(ctx context.Context) error
	Close() error
}

type Capabilities struct {
	TCP              bool
	UDP              bool
	TLSSNI           bool
	ProxyProtocolIn  bool
	ProxyProtocolOut bool
	TargetGroup      bool
	HostnameTarget   bool
	KernelL4         bool
}

type Status struct {
	Dataplane        string
	Available        bool
	Version          string
	Health           string
	Owner            string
	DriftStatus      string
	ExternalResource string
	Error            string
	LastApplyHash    string
}

type Options struct {
	Mode             string
	ConflictPolicy   string
	InstanceID       string
	ServiceName      string
	InstallDir       string
	StateDir         string
	RunDir           string
	HAProxyPath      string
	NFTPath          string
	CommandRunner    CommandRunner
	ExternalBackends bool
}

type CommandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
	Start(ctx context.Context, name string, args ...string) error
}

func normalizeMode(value string, fallback string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case ModeAuto:
		return ModeAuto
	case ModeHAProxy:
		return ModeHAProxy
	case ModeNFTables:
		return ModeNFTables
	case ModeNative:
		return ModeNative
	default:
		if strings.TrimSpace(fallback) != "" {
			return normalizeMode(fallback, ModeNative)
		}
		return ModeNative
	}
}

func normalizePreference(value string) string {
	return normalizeMode(value, ModeAuto)
}

func normalizeConflictPolicy(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case ConflictPolicyFailFast:
		return ConflictPolicyFailFast
	default:
		return ConflictPolicyFailFast
	}
}
