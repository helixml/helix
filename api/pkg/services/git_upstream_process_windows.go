package services

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

func configureUpstreamGitProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
