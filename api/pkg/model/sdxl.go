package model

import (
	"context"
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type SDXL struct {
}

func (l *SDXL) GetPrompt(ctx context.Context, session *types.Session) (string, error) {
	if len(session.Interactions) == 0 {
		return "", fmt.Errorf("session has no messages")
	}
	lastMessage := session.Interactions[len(session.Interactions)-1]
	return lastMessage.Message, nil
}

// Compile-time interface check:
var _ ImageModel = (*SDXL)(nil)
