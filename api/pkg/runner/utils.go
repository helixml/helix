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

func logAllProcesses() {
	cmd := exec.Command("ps", "auxwww")
	output, err := cmd.Output()
	if err != nil {
		log.Error().Err(err).Msg("Failed to execute ps auxwwww")
		return
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "<defunct>") {
			log.Info().Str("process", strings.TrimSpace(line)).Msg("Procs: ")
		}
	}
}

func killProcessTree(pid int) error {
	log.Info().Int("pid", pid).Msg("Starting killProcessTree")

	logAllProcesses()
	descendants, err := getAllDescendants(pid)
	if err != nil {
		log.Error().Err(err).Int("pid", pid).Msg("Failed to get descendants")
		return err
	}

	log.Info().Int("pid", pid).Ints("descendants", descendants).Msg("Got all descendants")

	// Add the original PID to the list
	allPids := append(descendants, pid)
	log.Info().Ints("all_pids", allPids).Msg("All PIDs to be terminated")

	// First, try to terminate gracefully with SIGINT
	for _, p := range allPids {
		log.Info().Int("pid", p).Msg("Sending SIGINT to process")
		err := syscall.Kill(p, syscall.SIGINT)
		if err != nil {
			log.Warn().Err(err).Int("pid", p).Msg("Failed to send SIGINT")
		}
		time.Sleep(10 * time.Second)
	}

	// Wait for processes to exit, or send SIGTERM after 10 seconds
	sigintTimeout := time.After(10 * time.Second)
	sigtermTimeout := time.After(40 * time.Second) // 10 seconds for SIGINT + 30 seconds for SIGTERM
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigintTimeout:
			logAllProcesses()
			log.Warn().Msg("SIGINT timeout reached, sending SIGTERM to remaining processes")
			for _, p := range allPids {
				if err := syscall.Kill(p, 0); err == nil {
					log.Info().Int("pid", p).Msg("Sending SIGTERM to process")
					err := syscall.Kill(p, syscall.SIGTERM)
					if err != nil {
						log.Warn().Err(err).Int("pid", p).Msg("Failed to send SIGTERM")
					}
					time.Sleep(10 * time.Second)
				}
			}
		case <-sigtermTimeout:
			logAllProcesses()
			log.Warn().Msg("SIGTERM timeout reached, force killing remaining processes")
			for _, p := range allPids {
				if err := syscall.Kill(p, 0); err == nil {
					log.Info().Int("pid", p).Msg("Sending SIGKILL to process")
					err := syscall.Kill(p, syscall.SIGKILL)
					if err != nil {
						log.Warn().Err(err).Int("pid", p).Msg("Failed to send SIGKILL")
					}
					time.Sleep(10 * time.Second)
				}
			}
			log.Info().Msg("All processes should be terminated now")
			return nil
		case <-ticker.C:
			remainingPids := []int{}
			for _, p := range allPids {
				if err := syscall.Kill(p, 0); err == nil {
					remainingPids = append(remainingPids, p)
				}
			}
			if len(remainingPids) == 0 {
				log.Info().Ints("all_pids", allPids).Msg("All processes have exited")
				return nil
			}
			log.Info().Ints("remaining_pids", remainingPids).Msg("Processes still running")
		}
	}
}
