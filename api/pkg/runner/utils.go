//go:build !windows
// +build !windows

package runner

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/shirou/gopsutil/process"
)

func getChildPids(pid int) ([]int, error) {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Command exited with non-zero exit code
			exitCode := exitErr.ExitCode()
			// Handle the exit code as needed
			if exitCode == 1 {
				// this CAN mean pgrep just found no matches, this just means no children
				return []int{}, nil
			} else {
				return nil, fmt.Errorf("error calling pgrep -P %d: %s, %s", pid, err, out)
			}
		} else {
			return nil, fmt.Errorf("error calling pgrep -P %d: %s, %s", pid, err, out)
		}
	}

	var pids []int
	for _, pidStr := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pidStr != "" {
			pid, _ := strconv.Atoi(pidStr)
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func getAllDescendants(pid int) ([]int, error) {
	children, err := getChildPids(pid)
	if err != nil {
		return nil, err
	}

	// nolint:prealloc
	// NOTE: we don't know the size
	var descendants []int

	for _, child := range children {
		descendants = append(descendants, child)
		grandchildren, err := getAllDescendants(child)
		if err != nil {
			return nil, err
		}
		descendants = append(descendants, grandchildren...)
	}

	return descendants, nil
}

func killProcessTree(pid int) error {
	descendants, err := getAllDescendants(pid)
	if err != nil {
		return err
	}

	// Add the original PID to the list
	allPids := append(descendants, pid)

	// First, try to terminate gracefully
	for _, p := range allPids {
		syscall.Kill(p, syscall.SIGTERM)
	}

	// Wait for processes to exit, or force kill after timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Force kill any remaining processes
			for _, p := range allPids {
				syscall.Kill(p, syscall.SIGKILL)
			}
			return nil
		case <-ticker.C:
			allExited := true
			for _, p := range allPids {
				if err := syscall.Kill(p, 0); err == nil {
					allExited = false
					break
				}
			}
			if allExited {
				return nil
			}
		}
	}
}

func getPidStatus(pid int) (string, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return "", err
	}

	stat, err := p.Status()
	if err != nil {
		return "", err
	}

	return stat, nil
}

//go:generate mockgen -source $GOFILE -destination utils_mocks.go -package $GOPACKAGE
type FreePortFinder interface {
	GetFreePort() (int, error)
}

type RealFreePortFinder struct{}

func (f *RealFreePortFinder) GetFreePort() (int, error) {
	return freeport.GetFreePort()
}

var freePortFinder FreePortFinder = &RealFreePortFinder{}
