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

func (l *SDXL) GetTask(session *types.Session) (*types.RunnerTask, error) {
	return getGenericTask(session)
}

func (l *SDXL) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	progressActivationWord := ""
	if mode == types.SessionModeFinetune {
		progressActivationWord = "steps:"
	}
	// the same chunker works for both modes
	// the same chunker handles both stdout and stderr
	// that is because the progress appears on stderr
	// and the session ID appears on stdout and so we need shared state
	// effectively - the chunker is getting a combo of stdout and stderr
	chunker := newSDXLChunker(eventHandler, SDXLChunkerOptions{
		progressActivationWord: progressActivationWord,
	})
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
		fmt.Sprintf("HELIX_NEXT_TASK_URL=%s", config.NextTaskURL),
		fmt.Sprintf("HELIX_INITIAL_SESSION_URL=%s", config.InitialSessionURL),
		"PYTHONUNBUFFERED=1",
	}

	return cmd, nil
}

type SDXLChunkerOptions struct {
	// if defined - we must wait until we see this word
	// before we start to activate percentages
	// this is because the fine tuning emits percentages
	// before the actual training starts so causes
	// the loading bar to flicker back and forth
	progressActivationWord string
}

// the same chunker works for inference and fine tuning
type SDXLChunker struct {
	sessionID      string
	progressActive bool
	options        SDXLChunkerOptions
	eventHandler   WorkerEventHandler
}

func newSDXLChunker(eventHandler WorkerEventHandler, options SDXLChunkerOptions) *SDXLChunker {
	return &SDXLChunker{
		sessionID:      "",
		progressActive: false,
		options:        options,
		eventHandler:   eventHandler,
	}
}

func (chunker *SDXLChunker) emitProgress(progress int) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeProgress,
		SessionID: chunker.sessionID,
		Progress:  progress,
	})
}

func (chunker *SDXLChunker) emitResult(files []string) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Files:     files,
	})
}

func (chunker *SDXLChunker) emitLora(loraDir string) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		LoraDir:   loraDir,
		Files:     []string{},
	})
}

func (chunker *SDXLChunker) write(word string) error {
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
	} else if strings.HasPrefix(word, "[SESSION_END_IMAGES]") {
		// e.g. [SESSION_END_IMAGES]images=["/home/kai/projects/helix/sd-scripts/./output_images/image_98f3af8a-f77f-4f49-8a26-6ae314a09d3d_20231116-135033_000.png"]
		parts := strings.Split(word, "=")
		var files []string
		err := json.Unmarshal([]byte(parts[1]), &files)
		if err != nil {
			return err
		}
		chunker.emitResult(files)
		chunker.reset()
	} else if strings.HasPrefix(word, "[SESSION_END_LORA_DIR]") {
		// e.g. [SESSION_END_LORA_DIR]lora_dir=/tmp/helix/results/123
		parts := strings.Split(word, "=")
		chunker.emitLora(parts[1])
		chunker.reset()
	} else if chunker.sessionID != "" {
		if chunker.options.progressActivationWord != "" && !chunker.progressActive && word == chunker.options.progressActivationWord {
			chunker.progressActive = true
		}
		// 10%|█
		if strings.Contains(word, "%|") && (chunker.options.progressActivationWord == "" || chunker.progressActive) {
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

func (chunker *SDXLChunker) reset() {
	chunker.sessionID = ""
	chunker.progressActive = false
}

// Compile-time interface check:
var _ Model = (*SDXL)(nil)
