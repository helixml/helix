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

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// get hackily written to at process startup
// TODO: remove this when we rip out cog downloading its weights over http
var API_HOST string
var API_TOKEN string

/*

Plan to integrate cog.

We need to rip the interaction code out of sd-scripts, and copy the behaviour.

*/

type CogSDXL struct {
}

func (l *CogSDXL) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 24
	} else {
		return MB * 19334
	}
}

func (l *CogSDXL) GetType() types.SessionType {
	return types.SessionTypeImage
}

func (l *CogSDXL) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	task.DatasetDir = fileManager.GetFolder()

	return task, nil
}

func (l *CogSDXL) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	progressActivationWord := ""
	if mode == types.SessionModeFinetune {
		progressActivationWord = ":step:"
	}
	// the same chunker works for both modes
	// the same chunker handles both stdout and stderr
	// that is because the progress appears on stderr
	// and the session ID appears on stdout and so we need shared state
	// effectively - the chunker is getting a combo of stdout and stderr
	chunker := newCogSDXLChunker(eventHandler, CogSDXLChunkerOptions{
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

func (l *CogSDXL) getMockCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {
		args := []string{
			"runner/sdxl_inference.py",
		}
		cmd = exec.CommandContext(
			ctx,
			"python",
			args...,
		)
	} else if sessionFilter.Mode == types.SessionModeFinetune {
		args := []string{
			"runner/sdxl_finetune.py",
		}
		cmd = exec.CommandContext(
			ctx,
			"python",
			args...,
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
		fmt.Sprintf("HELIX_MOCK_ERROR=%s", config.MockRunnerError),
		fmt.Sprintf("HELIX_MOCK_DELAY=%d", config.MockRunnerDelay),
		"PYTHONUNBUFFERED=1",
	}

	return cmd, nil
}

func (l *CogSDXL) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	var err error
	if isInitialSession && session.Mode == types.SessionModeInference && session.LoraDir != "" {
		session, err = downloadLoraDir(session, fileManager)
		if err != nil {
			return nil, err
		}
	}

	// download all files across all interactions
	// and accumulate them in the last user interaction
	if session.Mode == types.SessionModeFinetune {
		userInteractions := data.FilterUserInteractions(session.Interactions)
		finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)

		allFiles := []string{}

		for _, interaction := range finetuneInteractions {
			if interaction.Files != nil {
				allFiles = append(allFiles, interaction.Files...)
			}
		}

		for _, file := range allFiles {
			localPath := path.Join(fileManager.GetFolder(), path.Base(file))
			err := fileManager.DownloadFile(file, localPath)
			if err != nil {
				return nil, err
			}
		}
	}

	return session, nil
}

func (l *CogSDXL) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	if config.MockRunner {
		return l.getMockCommand(ctx, sessionFilter, config)
	}
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {
		args := []string{
			"runner/venv_command.sh",
			"python3", "-u",
			"helix_cog_wrapper.py", "inference",
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
			"python3", "-u",
			"helix_cog_wrapper.py", "finetune",
		)
	} else {
		return nil, fmt.Errorf("invalid session mode: %s", sessionFilter.Mode)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "cog-sdxl"))),
		fmt.Sprintf("HELIX_NEXT_TASK_URL=%s", config.NextTaskURL),
		fmt.Sprintf("HELIX_INITIAL_SESSION_URL=%s", config.InitialSessionURL),
		// cog likes to download LoRA from a URL, so we construct one for it
		fmt.Sprintf("API_HOST=%s", API_HOST),
		// one day it will need to auth to the API server to download LoRAs
		fmt.Sprintf("API_TOKEN=%s", API_TOKEN),
		"PYTHONUNBUFFERED=1",
	}

	return cmd, nil
}

type CogSDXLChunkerOptions struct {
	// if defined - we must wait until we see this word
	// before we start to activate percentages
	// this is because the fine tuning emits percentages
	// before the actual training starts so causes
	// the loading bar to flicker back and forth
	progressActivationWord string
}

// the same chunker works for inference and fine tuning
type CogSDXLChunker struct {
	sessionID      string
	progressActive bool
	options        CogSDXLChunkerOptions
	eventHandler   WorkerEventHandler
}

func newCogSDXLChunker(eventHandler WorkerEventHandler, options CogSDXLChunkerOptions) *CogSDXLChunker {
	return &CogSDXLChunker{
		sessionID:      "",
		progressActive: false,
		options:        options,
		eventHandler:   eventHandler,
	}
}

func (chunker *CogSDXLChunker) emitProgress(progress int) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeProgress,
		SessionID: chunker.sessionID,
		Progress:  progress,
	})
}

func (chunker *CogSDXLChunker) emitResult(files []string) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Files:     files,
	})
}

func (chunker *CogSDXLChunker) emitLora(loraDir string) {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		LoraDir:   loraDir,
		Files:     []string{},
	})
}

func (chunker *CogSDXLChunker) write(word string) error {
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

func (chunker *CogSDXLChunker) reset() {
	chunker.sessionID = ""
	chunker.progressActive = false
}

// Compile-time interface check:
var _ Model = (*CogSDXL)(nil)
