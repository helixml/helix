package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type OllamaGenericText struct {
	id            string // e.g. "phi3.5:3.8b-mini-instruct-q8_0"
	name          string // e.g. "Phi 3.5"
	memory        uint64
	contextLength int64
	description   string
	hide          bool
}

func (i *OllamaGenericText) GetMemoryRequirements(mode types.SessionMode) uint64 {
	return i.memory
}

func (i *OllamaGenericText) GetContextLength() int64 {
	return i.contextLength
}

func (i *OllamaGenericText) GetType() types.SessionType {
	return types.SessionTypeText
}

func (i *OllamaGenericText) GetID() string {
	return i.id
}

func (i *OllamaGenericText) ModelName() ModelName {
	return NewModel(i.id)
}

// TODO(rusenask): probably noop
func (i *OllamaGenericText) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (i *OllamaGenericText) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented 1")
}

func (i *OllamaGenericText) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented 2")
}

func (i *OllamaGenericText) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented 3")
}

func (i *OllamaGenericText) GetDescription() string {
	return i.description
}

func (i *OllamaGenericText) GetHumanReadableName() string {
	return i.name
}

func (i *OllamaGenericText) GetHidden() bool {
	return i.hide
}
