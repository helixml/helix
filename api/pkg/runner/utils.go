package runner

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	var descendants []int
	children, err := getChildPids(pid)
	if err != nil {
		return nil, err
	}

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
