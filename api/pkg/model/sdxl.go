package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type SDXL struct {
}

func (l *SDXL) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 24
	} else {
		return GB * 15
	}
}

func (l *SDXL) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (l *SDXL) GetTask(session *types.Session) (*types.WorkerTask, error) {
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
		Prompt: lastInteraction.Message,
	}, nil
}

func (l *SDXL) GetTextStream(mode types.SessionMode) (*TextStream, error) {
	return nil, nil
}

func (l *SDXL) GetCommand(ctx context.Context, mode types.SessionMode, config types.RunnerProcessConfig) (*exec.Cmd, error) {
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
			"accelerate", "launch",
			"--num_cpu_threads_per_process", "1",
			"sdxl_minimal_inference.py",
			"--ckpt_path=sdxl/sd_xl_base_1.0.safetensors",
			"--output_dir=./output_images",
		)

		cmd.Env = []string{
			fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "sd-scripts"))),
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
var _ Model = (*SDXL)(nil)
