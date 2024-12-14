package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

var _ Model = &DiffusersGenericImage{}

type DiffusersGenericImage struct {
	Id          string // e.g. "stabilityai/stable-diffusion-3.5-medium"
	Name        string // e.g. "Stable Diffusion 3.5 Medium"
	Memory      uint64
	Description string
	Hide        bool
}

func (i *DiffusersGenericImage) GetMemoryRequirements(mode types.SessionMode) uint64 {
	return i.Memory
}

func (i *DiffusersGenericImage) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (i *DiffusersGenericImage) GetID() string {
	return i.Id
}

func (i *DiffusersGenericImage) ModelName() ModelName {
	return NewModel(i.Id)
}

func (i *DiffusersGenericImage) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (i *DiffusersGenericImage) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented 1")
}

func (i *DiffusersGenericImage) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented 2")
}

func (i *DiffusersGenericImage) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented 3")
}

func (i *DiffusersGenericImage) GetDescription() string {
	return i.Description
}

func (i *DiffusersGenericImage) GetHumanReadableName() string {
	return i.Name
}

func (i *DiffusersGenericImage) GetHidden() bool {
	return i.Hide
}
