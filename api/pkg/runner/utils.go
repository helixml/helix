package runner

import (
	"time"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/process"
)

func killProcessTree(pid int) error {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}

	// First, try to terminate gracefully, ignore errors for now
	err = p.Terminate()
	if err != nil {
		return err
	}

	// Wait for processes to exit, or force kill after timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Warn().Msgf("having to force kill process tree for PIDs: %v", pid)
			// Force kill any remaining processes
			err = p.Kill()
			if err != nil {
				return err
			}
			return nil
		case <-ticker.C:
			isRunning, err := p.IsRunning()
			if err != nil {
				return err
			}
			if !isRunning {
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
