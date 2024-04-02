package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type OllamaGenericText struct {
	name   string
	memory uint64
}

func NewOllamaGenericText(name string, memory uint64) *OllamaGenericText {
	return &OllamaGenericText{name: name, memory: memory}
}

func (i *OllamaGenericText) GetMemoryRequirements(mode types.SessionMode) uint64 {
	return i.memory
}

func (i *OllamaGenericText) GetType() types.SessionType {
	return types.SessionTypeText
}

func (i *OllamaGenericText) ModelName() types.ModelName {
	return types.NewModel(i.name)
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
	return nil, fmt.Errorf("not implemented")
}

func (i *OllamaGenericText) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (i *OllamaGenericText) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented")
}
