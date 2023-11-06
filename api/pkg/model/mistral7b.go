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
	return getGenericTask(session)
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

func (l *Mistral7bInstruct01) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {

		// this bash script will be in the dockerfile that we use to
		// manage runners
		// TODO: should this be included in the gofs and written to the FS dynamically
		// so we can distribute a go binary if needed?
		cmd = exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"python", "-u", "-m",
			"axolotl.cli.inference",
			"examples/mistral/qlora-instruct.yml",
		)
	} else {
		return nil, fmt.Errorf("invalid session mode: %s", sessionFilter.Mode)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
		fmt.Sprintf("HELIX_GET_JOB_URL=%s", config.TaskURL),
		fmt.Sprintf("HELIX_RESPOND_JOB_URL=%s", config.ResponseURL),
	}

	return cmd, nil
}

// Compile-time interface check:
var _ Model = (*Mistral7bInstruct01)(nil)
