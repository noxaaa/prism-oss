package dataplane

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/forward"
)

type NativeBackend struct {
	supervisor *forward.Supervisor
}

func (backend *NativeBackend) Name() string {
	return ModeNative
}

func (backend *NativeBackend) Capabilities() Capabilities {
	return Capabilities{
		TCP:              true,
		UDP:              true,
		TLSSNI:           true,
		ProxyProtocolIn:  true,
		ProxyProtocolOut: true,
		TargetGroup:      true,
		HostnameTarget:   true,
	}
}

func (backend *NativeBackend) Apply(ctx context.Context, snapshot agent.ConfigSnapshot) error {
	return backend.supervisor.Apply(ctx, snapshot)
}

func (backend *NativeBackend) AgentMetrics() agent.MetricsPayload {
	return backend.supervisor.AgentMetrics()
}

func (backend *NativeBackend) Status(context.Context) Status {
	return Status{Dataplane: ModeNative, Available: true, Health: "HEALTHY"}
}

func (backend *NativeBackend) CleanupOwnedState(ctx context.Context) error {
	return backend.supervisor.Apply(ctx, agent.ConfigSnapshot{})
}

func (backend *NativeBackend) Close() error {
	backend.supervisor.Close()
	return nil
}
