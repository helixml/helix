package runner

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/process"
)

func getAllDescendants(p *process.Process) ([]*process.Process, error) {
	var descendants []*process.Process
	children, err := p.Children()
	if err != nil {
		if errors.Is(err, process.ErrorNoChildren) {
			return descendants, nil
		}
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
	parent, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}

	descendants, err := getAllDescendants(parent)
	if err != nil {
		return err
	}

	// First kill all the children
	for _, p := range descendants {
		err := p.Terminate()
		if err != nil {
			log.Error().Err(err).Msgf("failed to terminate process %d", p.Pid)
		}
	}

	// Then terminate the parent
	err = parent.Terminate()
	if err != nil {
		log.Error().Err(err).Msgf("failed to terminate process %d", pid)
	}

	// Wait for processes to exit, or force kill after timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Warn().Msgf("having to force kill process tree for PIDs")
			// Force kill any remaining processes
			for _, p := range descendants {
				running, err := p.IsRunning()
				if err != nil {
					log.Error().Err(err).Msgf("failed to check if process %d is running", p.Pid)
				}
				if running {
					err := p.Kill()
					if err != nil {
						log.Error().Err(err).Msgf("failed to kill process %d", p.Pid)
					}
				}
			}
			// And force kill the parent
			running, err := parent.IsRunning()
			if err != nil {
				log.Error().Err(err).Msgf("failed to check if process %d is running", pid)
			}
			if running {
				err = parent.Kill()
				if err != nil {
					log.Error().Err(err).Msgf("failed to kill process %d", pid)
				}
			}
			return nil
		case <-ticker.C:
			allExited := true
			for _, p := range descendants {
				running, err := p.IsRunning()
				if err != nil {
					log.Error().Err(err).Msgf("failed to check if process %d is running", p.Pid)
				}
				if running {
					allExited = false
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
