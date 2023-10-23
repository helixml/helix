package model

import (
	"context"
	"fmt"

	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type SDXL struct {
}

func (l *SDXL) GetMemoryUsage(ctx context.Context) (uint64, error) {
	b, err := bytesize.Parse("12GB")
	if err != nil {
		return 0, err
	}
	return uint64(b), err
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
var _ Model = (*SDXL)(nil)
