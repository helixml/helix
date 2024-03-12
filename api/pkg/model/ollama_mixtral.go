package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type OllamaMixtral struct{}

func (i *OllamaMixtral) GetMemoryRequirements(mode types.SessionMode) uint64 {
	return GB * 24
}

func (i *OllamaMixtral) GetType() types.SessionType {
	return types.SessionTypeText
}

// TODO(rusenask): probably noop
func (i *OllamaMixtral) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (i *OllamaMixtral) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented")
}

func (i *OllamaMixtral) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (i *OllamaMixtral) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented")
}
