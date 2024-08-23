package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/rs/zerolog/log"
)

func killProcessTree(pid int) error {
	log.Debug().Int("pid", pid).Msg("Entering killProcessTree function")

	// Send ctrl+c equivalent signal, which hopefully makes the process clean up
	// all its children
	log.Debug().Int("pid", pid).Msg("Sending SIGINT to process")
	err := syscall.Kill(pid, syscall.SIGINT)
	if err != nil {
		log.Error().Err(err).Int("pid", pid).Msg("Failed to send SIGINT to process")
		return fmt.Errorf("failed to send SIGINT to process %d: %w", pid, err)
	}
	log.Debug().Int("pid", pid).Msg("Successfully sent SIGINT to process")

	// Wait for the process and its descendants to exit, or timeout after 30 seconds
	log.Debug().Int("pid", pid).Msg("Starting wait loop for process and descendants")
	panicTimeout := time.After(30 * time.Second)
	sigkillTimeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-panicTimeout:
			log.Warn().Int("pid", pid).Msg("Timed out waiting for process and descendants to exit")
			panic(fmt.Sprintf(
				"Timed out waiting for process %d and its descendants to exit. "+
					"No idea if we have freed GPU memory at this point, "+
					"so exiting and hoping we get restarted",
				pid))
		case <-sigkillTimeout:
			log.Warn().Int("pid", pid).Msg("10 seconds passed, sending SIGKILL to process and descendants")
			descendants, _ := getDescendantPIDs(pid)
			for _, descendantPID := range descendants {
				log.Debug().Int("descendant_pid", descendantPID).Msg("Sending SIGKILL to descendant")
				_ = syscall.Kill(descendantPID, syscall.SIGKILL)
			}
			log.Debug().Int("pid", pid).Msg("Sending SIGKILL to main process")
			_ = syscall.Kill(pid, syscall.SIGKILL)
			// Continue waiting for processes to exit
		case <-ticker.C:
			log.Debug().Int("pid", pid).Msg("Checking if process and descendants still exist")
			// Check if the process and its descendants still exist
			descendants, err := getDescendantPIDs(pid)
			if err != nil {
				log.Debug().Err(err).Int("pid", pid).Msg("Failed to get descendants, assuming process has exited")
				// If we can't get descendants, assume the process has exited
				return nil
			}
			log.Debug().Int("pid", pid).Str("descendants", fmt.Sprintf("%v", descendants)).Msg("Found descendants")

			if len(descendants) == 0 {
				log.Info().Int("pid", pid).Msg("All processes have exited")
				// All processes have exited
				return nil
			}
			log.Debug().Int("pid", pid).Msg("Some processes are still running, continuing to wait")
		}
	}
}

func getDescendantPIDs(pid int) ([]int, error) {
	log.Debug().Int("pid", pid).Msg("Entering getDescendantPIDs function")

	cmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	log.Debug().Int("pid", pid).Str("command", cmd.String()).Msg("Executing pgrep command")
	output, err := cmd.Output()
	if err != nil {
		// Check if the error is due to no matching processes (exit status 1)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			log.Debug().Int("pid", pid).Msg("No child processes found")
			return []int{}, nil
		}
		log.Error().Err(err).Int("pid", pid).Msg("Failed to execute pgrep command")
		return nil, err
	}

	var pids []int
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		childPID, err := strconv.Atoi(line)
		if err != nil {
			log.Warn().Err(err).Str("line", line).Msg("Failed to convert PID to integer, skipping")
			continue
		}
		pids = append(pids, childPID)
		log.Debug().Int("child_pid", childPID).Msg("Found child process")

		// Recursively get descendants of this child
		descendants, err := getDescendantPIDs(childPID)
		if err == nil {
			pids = append(pids, descendants...)
			log.Debug().Int("child_pid", childPID).Int("descendant_count", len(descendants)).Msg("Found descendants for child process")
		} else {
			log.Warn().Err(err).Int("child_pid", childPID).Msg("Failed to get descendants for child process")
		}
	}

	// Check if the original process still exists
	log.Debug().Int("pid", pid).Msg("Checking if original process still exists")
	process, err := os.FindProcess(pid)
	if err == nil {
		// On Unix, FindProcess always succeeds, so we need to send signal 0 to test existence
		err = process.Signal(syscall.Signal(0))
		if err == nil {
			pids = append(pids, pid)
			log.Debug().Int("pid", pid).Msg("Original process still exists")
		} else {
			log.Debug().Int("pid", pid).Msg("Original process no longer exists")
		}
	} else {
		log.Debug().Int("pid", pid).Msg("Failed to find process")
	}

	log.Debug().Int("pid", pid).Int("total_pids", len(pids)).Msg("Exiting getDescendantPIDs function")
	return pids, nil
}

//go:generate mockgen -source $GOFILE -destination utils_mocks.go -package $GOPACKAGE
type FreePortFinder interface {
	GetFreePort() (int, error)
}

type RealFreePortFinder struct{}

func (f *RealFreePortFinder) GetFreePort() (int, error) {
	log.Debug().Msg("Getting free port")
	port, err := freeport.GetFreePort()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get free port")
	} else {
		log.Debug().Int("port", port).Msg("Successfully got free port")
	}
	return port, err
}

var freePortFinder FreePortFinder = &RealFreePortFinder{}
