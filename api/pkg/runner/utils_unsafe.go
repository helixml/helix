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

	"github.com/rs/zerolog/log"
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
			}
			return nil, fmt.Errorf("error calling pgrep -P %d: %s, %s", pid, err, out)
		}
		return nil, fmt.Errorf("error calling pgrep -P %d: %s, %s", pid, err, out)
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
	log.Info().Int("root_pid", pid).Msg("PROCESS_CLEANUP: Starting process tree cleanup")

	descendants, err := getAllDescendants(pid)
	if err != nil {
		log.Error().Err(err).Int("root_pid", pid).Msg("PROCESS_CLEANUP: Failed to get descendants")
		return err
	}

	// Add the original PID to the list
	allPids := append(descendants, pid)

	log.Info().Interface("all_pids", allPids).Int("total_processes", len(allPids)).Msg("PROCESS_CLEANUP: Found processes to kill")

	// First, try to terminate gracefully
	log.Info().Interface("pids", allPids).Msg("PROCESS_CLEANUP: Sending SIGTERM to all processes")
	for _, p := range allPids {
		if err := syscall.Kill(p, syscall.SIGTERM); err != nil {
			log.Error().Err(err).Int("pid", p).Msg("PROCESS_CLEANUP: Failed to send SIGTERM to process")
		}
	}

	// Also try to kill the process group of the main process as a backup
	// This handles cases where child processes are in the same process group
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		log.Warn().Err(err).Int("pgid", pid).Msg("PROCESS_CLEANUP: Failed to send SIGTERM to process group (this may be normal)")
	} else {
		log.Info().Int("pgid", pid).Msg("PROCESS_CLEANUP: Sent SIGTERM to process group")
	}

	// Wait for processes to exit, or force kill after timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Warn().Msg("PROCESS_CLEANUP: Timeout reached, force killing remaining processes")
			// Force kill any remaining processes
			for _, p := range allPids {
				log.Info().Int("pid", p).Msg("PROCESS_CLEANUP: Force killing process with SIGKILL")
				if err := syscall.Kill(p, syscall.SIGKILL); err != nil {
					log.Error().Err(err).Int("pid", p).Msg("PROCESS_CLEANUP: Failed to send SIGKILL to process")
				}
			}

			// Also force kill the process group as final cleanup
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
				log.Warn().Err(err).Int("pgid", pid).Msg("PROCESS_CLEANUP: Failed to send SIGKILL to process group")
			} else {
				log.Info().Int("pgid", pid).Msg("PROCESS_CLEANUP: Sent SIGKILL to process group")
			}

			log.Info().Msg("PROCESS_CLEANUP: Force kill completed")

			// Final verification - check if any processes are still alive
			stillAlive := []int{}
			for _, p := range allPids {
				if err := syscall.Kill(p, 0); err == nil {
					stillAlive = append(stillAlive, p)
				}
			}
			if len(stillAlive) > 0 {
				log.Error().Interface("still_alive_pids", stillAlive).Msg("PROCESS_CLEANUP: WARNING - Some processes still alive after force kill!")
			} else {
				log.Info().Msg("PROCESS_CLEANUP: Verified all processes are dead")
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
				log.Info().Msg("PROCESS_CLEANUP: All processes exited successfully")
				return nil
			} else {
				log.Debug().Msg("PROCESS_CLEANUP: Some processes still alive, continuing to wait")
			}
		}
	}
}
