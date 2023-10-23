package model

import (
	"context"
	"fmt"

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

func (l *SDXL) GetPrompt(ctx context.Context, session *types.Session) (string, error) {
	if len(session.Interactions) == 0 {
		return "", fmt.Errorf("session has no messages")
	}
	lastMessage := session.Interactions[len(session.Interactions)-1]
	return lastMessage.Message, nil
}

func (l *SDXL) GetTextStream(ctx context.Context) (*TextStream, error) {
	return nil, nil
}

func (l *SDXL) RunProcess(mode types.SessionMode) error {
	return nil
}

// Compile-time interface check:
var _ Model = (*SDXL)(nil)
