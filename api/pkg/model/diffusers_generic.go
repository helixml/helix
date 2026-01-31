package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

var _ Model = &DiffusersGenericImage{}

type DiffusersGenericImage struct {
	ID          string // e.g. "stabilityai/stable-diffusion-3.5-medium"
	Name        string // e.g. "Stable Diffusion 3.5 Medium"
	Memory      uint64
	Description string
	Hide        bool
	Prewarm     bool // Whether to prewarm this model (usually false for image models due to high memory usage)
}

func (i *DiffusersGenericImage) GetMemoryRequirements(_ types.SessionMode) uint64 {
	return i.Memory
}

func (i *DiffusersGenericImage) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (i *DiffusersGenericImage) GetContextLength() int64 {
	return 0 // Default to 0 (use model's default)
}

func (i *DiffusersGenericImage) GetConcurrency() int {
	return 0 // Default to 0 (use runtime default)
}

func (i *DiffusersGenericImage) GetID() string {
	return i.ID
}

func (i *DiffusersGenericImage) ModelName() Name {
	return NewModel(i.ID)
}

func (i *DiffusersGenericImage) GetTask(session *types.Session, _ SessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (i *DiffusersGenericImage) GetCommand(_ context.Context, _ types.SessionFilter, _ types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented 1")
}

func (i *DiffusersGenericImage) GetTextStreams(_ types.SessionMode, _ WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented 2")
}

func (i *DiffusersGenericImage) PrepareFiles(_ *types.Session, _ bool, _ SessionFileManager) (*types.Session, error) {
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
