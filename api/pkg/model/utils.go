package model

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// define 1 GB as a uint64 number of bytes
const GB uint64 = 1024 * 1024 * 1024
const MB uint64 = 1024 * 1024

// each model get's to decide what it's task looks like
// but this is the vanilla "most models return this"
// version - models call this and are free to override fields
func getGenericTask(session *types.Session) (*types.RunnerTask, error) {
	if len(session.Interactions) == 0 {
		return nil, fmt.Errorf("session has no messages")
	}

	lastInteraction := session.Interactions[len(session.Interactions)-1]

	if lastInteraction == nil {
		return nil, fmt.Errorf("session has no user messages")
	}

	switch session.Mode {
	case types.SessionModeInference:
		return &types.RunnerTask{
			Prompt:  lastInteraction.PromptMessage,
			LoraDir: session.LoraDir,
		}, nil
	default:
		return nil, fmt.Errorf("invalid session mode")
	}
}
