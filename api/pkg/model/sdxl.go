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
	lastInteraction, err := GetUserInteraction(session)
	if err != nil {
		return nil, err
	}
	if lastInteraction == nil {
		return nil, fmt.Errorf("session has no user messages")
	}
	if session.Mode == types.SessionModeInference {
		return &types.WorkerTask{
			Prompt: lastInteraction.Message,
		}, nil
	} else if session.Mode == types.SessionModeFinetune {
		if len(lastInteraction.Files) == 0 {
			return nil, fmt.Errorf("session has no files")
		}
		// we expect all of the files to have been downloaded
		// by the controller and put into a shared folder
		// so - we extract the folder path from the first file
		// and pass it into the python job as the input dir
		return &types.WorkerTask{
			FinetuneInputDir: path.Dir(lastInteraction.Files[0]),
		}, nil
	} else {
		return nil, fmt.Errorf("invalid session mode")
	}
}

func (l *SDXL) GetTextStream(mode types.SessionMode) (*TextStream, error) {
	return nil, nil
}

func (l *SDXL) GetCommand(ctx context.Context, mode types.SessionMode, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if mode == types.SessionModeInference {
		cmd = exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"accelerate", "launch",
			"--num_cpu_threads_per_process", "1",
			"sdxl_minimal_inference.py",
			"--ckpt_path=sdxl/sd_xl_base_1.0.safetensors",
			"--output_dir=./output_images",
		)
	} else if mode == types.SessionModeFinetune {
		cmd = exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"accelerate", "launch",
			"--num_cpu_threads_per_process", "1",
			"sdxl_train_network.py",
			"--pretrained_model_name_or_path=./sdxl/sd_xl_base_1.0.safetensors",
			"--output_name=lora",
			"--save_model_as=safetensors",
			"--prior_loss_weight=1.0",
			"--max_train_steps=400",
			"--vae=madebyollin/sdxl-vae-fp16-fix",
			"--learning_rate=1e-4",
			"--optimizer_type=AdamW8bit",
			"--xformers",
			"--mixed_precision=fp16",
			"--cache_latents",
			"--gradient_checkpointing",
			"--save_every_n_epochs=1",
			"--network_module=networks.lora",
		)
	}

	if cmd == nil {
		return nil, fmt.Errorf("not implemented")
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "sd-scripts"))),
		fmt.Sprintf("HELIX_GET_JOB_URL=%s", config.TaskURL),
		fmt.Sprintf("HELIX_RESPOND_JOB_URL=%s", config.ResponseURL),
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd, nil
}

// Compile-time interface check:
var _ Model = (*SDXL)(nil)
