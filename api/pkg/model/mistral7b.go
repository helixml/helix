package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type Mistral7bInstruct01 struct {
}

func (l *Mistral7bInstruct01) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 24
	} else {
		return GB * 7
	}
}

func (l *Mistral7bInstruct01) GetType() types.SessionType {
	return types.SessionTypeText
}

func (l *Mistral7bInstruct01) GetTask(session *types.Session) (*types.WorkerTask, error) {
	if len(session.Interactions) == 0 {
		return nil, fmt.Errorf("session has no messages")
	}
	lastInteraction, err := getUserInteraction(session)

	if err != nil {
		return nil, err
	}

	if lastInteraction == nil {
		return nil, fmt.Errorf("session has no user messages")
	}

	return &types.WorkerTask{
		Prompt: fmt.Sprintf("[INST]%s[/INST]", lastInteraction.Message),
	}, nil
}

func (l *Mistral7bInstruct01) GetTextStream(mode types.SessionMode) (*TextStream, error) {
	if mode == types.SessionModeInference {
		return NewTextStream(
			splitOnSpace,
			"[/INST]",
			"</s>",
		), nil
	}
	return nil, nil
}

func (l *Mistral7bInstruct01) GetCommand(ctx context.Context, mode types.SessionMode, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	if mode == types.SessionModeInference {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		// this bash script will be in the dockerfile that we use to
		// manage runners
		// TODO: should this be included in the gofs and written to the FS dynamically
		// so we can distribute a go binary if needed?
		cmd := exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"python", "-u", "-m",
			"axolotl.cli.inference",
			"examples/mistral/qlora-instruct.yml",
		)

		cmd.Env = []string{
			fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
			fmt.Sprintf("HELIX_GET_JOB_URL=%s", config.TaskURL),
			fmt.Sprintf("HELIX_RESPOND_JOB_URL=%s", config.ResponseURL),
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		return cmd, nil
	}

	return nil, fmt.Errorf("not implemented")
}

// Compile-time interface check:
var _ Model = (*Mistral7bInstruct01)(nil)
