package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type OllamaGemma7bInstruct01 struct{}

func (i *OllamaGemma7bInstruct01) GetMemoryRequirements(mode types.SessionMode) uint64 {
	return MB * 7440
}

func (i *OllamaGemma7bInstruct01) GetType() types.SessionType {
	return types.SessionTypeText
}

// TODO: remove
func (i *OllamaGemma7bInstruct01) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

// TODO: remove
func (i *OllamaGemma7bInstruct01) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented")
}

// TODO: remove
func (i *OllamaGemma7bInstruct01) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

// TODO: remove
func (i *OllamaGemma7bInstruct01) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented")
}
