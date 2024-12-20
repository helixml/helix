package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type OllamaGenericText struct {
	Id            string // e.g. "phi3.5:3.8b-mini-instruct-q8_0"
	Name          string // e.g. "Phi 3.5"
	Memory        uint64
	ContextLength int64
	Description   string
	Hide          bool
}

func (i *OllamaGenericText) GetMemoryRequirements(_ types.SessionMode) uint64 {
	return i.Memory
}

func (i *OllamaGenericText) GetContextLength() int64 {
	return i.ContextLength
}

func (i *OllamaGenericText) GetType() types.SessionType {
	return types.SessionTypeText
}

func (i *OllamaGenericText) GetID() string {
	return i.Id
}

func (i *OllamaGenericText) ModelName() ModelName {
	return NewModel(i.Id)
}

// TODO(rusenask): probably noop
func (i *OllamaGenericText) GetTask(session *types.Session, _ ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (i *OllamaGenericText) GetCommand(_ context.Context, _ types.SessionFilter, _ types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented 1")
}

func (i *OllamaGenericText) GetTextStreams(_ types.SessionMode, _ WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented 2")
}

func (i *OllamaGenericText) PrepareFiles(_ *types.Session, _ bool, _ ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented 3")
}

func (i *OllamaGenericText) GetDescription() string {
	return i.Description
}

func (i *OllamaGenericText) GetHumanReadableName() string {
	return i.Name
}

func (i *OllamaGenericText) GetHidden() bool {
	return i.Hide
}
