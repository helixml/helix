package model

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type SDXL struct {
}

func (l *SDXL) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 24
	} else {
		return MB * 7500
	}
}

func (l *SDXL) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (l *SDXL) GetTask(session *types.Session) (*types.WorkerTask, error) {
	return getGenericTask(session)
}

func (l *SDXL) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	if mode == types.SessionModeInference {
		// the same chunker handles both stdout and stderr
		// that is because the progress appears on stderr
		// and the session ID appears on stdout and so we need shared state
		// effectively - the chunker is getting a combo of stdout and stderr
		chunker := newSDXLInferenceChunker(eventHandler)
		stdout := NewTextStream(bufio.ScanWords, func(chunk string) {
			err := chunker.write(chunk)
			if err != nil {
				log.Error().Msgf("error writing word to sdxl inference chunker: %s", err)
			}
		})
		stderr := NewTextStream(bufio.ScanWords, func(chunk string) {
			err := chunker.write(chunk)
			if err != nil {
				log.Error().Msgf("error writing word to sdxl inference chunker: %s", err)
			}
		})
		return stdout, stderr, nil
	}
	return nil, nil, nil
}

func (l *SDXL) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {
		args := []string{
			"runner/venv_command.sh",
			"accelerate", "launch",
			"--num_cpu_threads_per_process", "1",
			"sdxl_minimal_inference.py",
			"--ckpt_path=sdxl/sd_xl_base_1.0.safetensors",
			"--output_dir=./output_images",
		}

		cmd = exec.CommandContext(
			ctx,
			"bash",
			args...,
		)
	} else if sessionFilter.Mode == types.SessionModeFinetune {
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
	} else {
		return nil, fmt.Errorf("invalid session mode: %s", sessionFilter.Mode)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "sd-scripts"))),
		fmt.Sprintf("HELIX_GET_JOB_URL=%s", config.TaskURL),
		fmt.Sprintf("HELIX_READ_INITIAL_SESSION_URL=%s", config.SessionURL),
		fmt.Sprintf("HELIX_RESPOND_JOB_URL=%s", config.ResponseURL),
		"PYTHONUNBUFFERED=1",
	}

	return cmd, nil
}

type SDXLInferenceChunker struct {
	sessionID    string
	eventHandler WorkerEventHandler
}

func newSDXLInferenceChunker(eventHandler WorkerEventHandler) *SDXLInferenceChunker {
	return &SDXLInferenceChunker{
		sessionID:    "",
		eventHandler: eventHandler,
	}
}

func (chunker *SDXLInferenceChunker) emitProgress(progress int) {
	chunker.eventHandler(&types.WorkerTaskResponse{
		Type:      types.WorkerTaskResponseTypeProgress,
		SessionID: chunker.sessionID,
		Progress:  progress,
	})
}

func (chunker *SDXLInferenceChunker) emitResult(files []string) {
	chunker.eventHandler(&types.WorkerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Files:     files,
	})
}

func (chunker *SDXLInferenceChunker) write(word string) error {
	if strings.HasPrefix(word, "[SESSION_START]") {
		// [SESSION_START]session_id=7d11a9ef-a192-426c-bc8e-6bd2c6364b46
		parts := strings.Split(word, "=")
		if len(parts) < 2 {
			// we reset here because we got a session start line with no ID
			// which is very strange
			chunker.reset()
			return fmt.Errorf("invalid session start line: %s", word)
		}
		chunker.sessionID = parts[1]
	} else if strings.HasPrefix(word, "[SESSION_END]") {
		// e.g. [SESSION_END]["/home/kai/projects/helix/sd-scripts/./output_images/image_98f3af8a-f77f-4f49-8a26-6ae314a09d3d_20231116-135033_000.png"]
		data := strings.Replace(word, "[SESSION_END]", "", 1)
		var files []string
		err := json.Unmarshal([]byte(data), &files)
		if err != nil {
			return err
		}
		chunker.emitResult(files)
		chunker.reset()
	} else {
		// 10%|█
		if strings.Contains(word, "%|") {
			parts := strings.Split(word, "%")
			percentStr := parts[0]
			progress, err := strconv.Atoi(percentStr)
			if err != nil {
				return err
			}
			chunker.emitProgress(progress)
		}
	}
	return nil
}

func (chunker *SDXLInferenceChunker) reset() {
	chunker.sessionID = ""
}

// Compile-time interface check:
var _ Model = (*SDXL)(nil)
