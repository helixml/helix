package runner

import (
	"errors"

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

	err = parent.Terminate()
	if err != nil {
		log.Error().Err(err).Msgf("failed to terminate process %d", parent.Pid)
	}
	return nil
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
