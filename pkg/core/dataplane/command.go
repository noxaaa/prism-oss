package dataplane

import (
	"context"
	"os/exec"
)

type execCommandRunner struct{}

func (runner execCommandRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (runner execCommandRunner) Start(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
