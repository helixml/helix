package runner

import (
	"context"
	"os/exec"
)

// Commander is a wrapper around exec.CommandContext to allow for testing
//
//go:generate mockgen -source $GOFILE -destination commander_mocks.go -package $GOPACKAGE
type Commander interface {
	LookPath(file string) (string, error)
	CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd
}

type RealCommander struct{}

func (c *RealCommander) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (c *RealCommander) CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
