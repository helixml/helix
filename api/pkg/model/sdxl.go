package model

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type SDXL struct {
}

func (l *SDXL) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 12
	} else {
		return GB * 6
	}
}

func (l *SDXL) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (l *SDXL) GetTask(ctx context.Context, session *types.Session) (*types.WorkerTask, error) {
	if len(session.Interactions) == 0 {
		return nil, fmt.Errorf("session has no messages")
	}
	lastMessage := session.Interactions[len(session.Interactions)-1]
	return &types.WorkerTask{
		Prompt: lastMessage.Message,
	}, nil
}

func (l *SDXL) GetTextStream(ctx context.Context, mode types.SessionMode) (*TextStream, error) {
	return nil, nil
}

func (l *SDXL) GetCommand(ctx context.Context, mode types.SessionMode) (*exec.Cmd, error) {
	return nil, nil
}

// Compile-time interface check:
var _ Model = (*SDXL)(nil)
