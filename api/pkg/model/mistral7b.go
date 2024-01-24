package model

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Mistral7bInstruct01 struct {
}

func (l *Mistral7bInstruct01) GetMemoryRequirements(mode types.SessionMode) uint64 {
	if mode == types.SessionModeFinetune {
		return GB * 24
	} else {
		return MB * 6440
	}
}

func (l *Mistral7bInstruct01) GetType() types.SessionType {
	return types.SessionTypeText
}

func (l *Mistral7bInstruct01) GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}

	task.DatasetDir = fileManager.GetFolder()

	var messages []string
	for _, interaction := range session.Interactions {
		// Chat API mode
		// if len(interaction.Messages) > 0 {
		// 	for _, m := range interaction.Messages {
		// 		if m.Role == "user" {
		// 			messages = append(messages, fmt.Sprintf("[INST]%s[/INST]", m.Content))
		// 		} else {
		// 			messages = append(messages, m.Content)
		// 		}
		// 	}
		// 	continue
		// }

		// Regular session mode
		if interaction.Creator == "user" {
			messages = append(messages, fmt.Sprintf("[INST]%s[/INST]", interaction.Message))
		} else {
			messages = append(messages, interaction.Message)
		}
	}

	task.Prompt = strings.Join(messages, "\n") + "\n"
	return task, nil
}

func (l *Mistral7bInstruct01) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	if mode == types.SessionModeInference {
		// this understands the context of each word and keeps state
		// to manage the session output window and emit events
		// via the event handler
		chunker := newMistral7bInferenceChunker(eventHandler, mistral7bInferenceChunkerOptions{
			// no buffering - send every single word
			bufferSize: 0,
			mistral:    l,
		})

		// this will get called for each word
		stdout := NewTextStream(scanWordsPreserveNewlines, func(chunk string) {
			err := chunker.write(chunk)
			if err != nil {
				log.Error().Msgf("error writing word to mistral inference chunker: %s", err)
			}
		})

		return stdout, nil, nil
	} else if mode == types.SessionModeFinetune {
		chunker := newMistral7bFinetuneChunker(eventHandler, mistral7bFinetuneChunkerOptions{
			progressActivationWord: "[axolotl.load_model:562]",
		})
		stdout := NewTextStream(bufio.ScanWords, func(line string) {
			err := chunker.write(line)
			if err != nil {
				log.Error().Msgf("error writing word to mistral inference chunker: %s", err)
			}
		})
		stderr := NewTextStream(bufio.ScanWords, func(line string) {
			err := chunker.write(line)
			if err != nil {
				log.Error().Msgf("error writing word to mistral inference chunker: %s", err)
			}
		})
		return stdout, stderr, nil
	}

	return nil, nil, nil
}

func (l *Mistral7bInstruct01) PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error) {
	var err error
	if isInitialSession && session.Mode == types.SessionModeInference && session.LoraDir != "" {
		session, err = downloadLoraDir(session, fileManager)
		if err != nil {
			return nil, err
		}
	}

	// accumulate all JSONL files across all interactions
	// and append them to one large JSONL file
	if session.Mode == types.SessionModeFinetune {
		userInteractions := data.FilterUserInteractions(session.Interactions)
		finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)
		jsonLFiles := []string{}
		for _, interaction := range finetuneInteractions {
			for _, file := range interaction.Files {
				if path.Base(file) == types.TEXT_DATA_PREP_QUESTIONS_FILE {
					localFilename := fmt.Sprintf("%s.jsonl", interaction.ID)
					localPath := path.Join(fileManager.GetFolder(), localFilename)
					err := fileManager.DownloadFile(file, localPath)
					if err != nil {
						return nil, err
					}
					jsonLFiles = append(jsonLFiles, localPath)
				}
			}
		}

		combinedFile := path.Join(fileManager.GetFolder(), types.TEXT_DATA_PREP_QUESTIONS_FILE)
		err = system.ConcatenateFiles(combinedFile, jsonLFiles, "\n")
		if err != nil {
			return nil, err
		}
	}

	return session, nil
}

func (l *Mistral7bInstruct01) getMockCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {
		args := []string{
			"./runner/axolotl_inference.py",
		}
		cmd = exec.CommandContext(
			ctx,
			"python",
			args...,
		)
	} else {
		args := []string{
			"./runner/axolotl_finetune.py",
		}
		cmd = exec.CommandContext(
			ctx,
			"python",
			args...,
		)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		// inherit PATH set in docker image or elsewhere
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", os.Getenv("CUDA_VISIBLE_DEVICES")),
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
		fmt.Sprintf("HELIX_NEXT_TASK_URL=%s", config.NextTaskURL),
		fmt.Sprintf("HELIX_INITIAL_SESSION_URL=%s", config.InitialSessionURL),
		fmt.Sprintf("HELIX_MOCK_ERROR=%s", config.MockRunnerError),
		fmt.Sprintf("HELIX_MOCK_DELAY=%d", config.MockRunnerDelay),
	}

	return cmd, nil
}

func (l *Mistral7bInstruct01) GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error) {
	if config.MockRunner {
		return l.getMockCommand(ctx, sessionFilter, config)
	}
	var cmd *exec.Cmd
	if sessionFilter.Mode == types.SessionModeInference {
		cmd = exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"python", "-u", "-m",
			"axolotl.cli.inference",
			"helix-mistral-instruct-v1.yml",
		)
	} else {
		cmd = exec.CommandContext(
			ctx,
			"bash", "runner/venv_command.sh",
			"python", "-u", "-m",
			"axolotl.cli.train",
			"helix-mistral-instruct-v1.yml",
		)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", os.Getenv("CUDA_VISIBLE_DEVICES")),
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
		fmt.Sprintf("HELIX_NEXT_TASK_URL=%s", config.NextTaskURL),
		fmt.Sprintf("HELIX_INITIAL_SESSION_URL=%s", config.InitialSessionURL),
	}

	return cmd, nil
}

type mistral7bInferenceChunkerOptions struct {
	// the max size of our buffer - we emit an event if the buffer get's bigger than this
	bufferSize int
	// need to access turns: how many user requests (used to identify boundary between input and output)
	mistral *Mistral7bInstruct01
}

type mistral7bInferenceChunker struct {
	options   mistral7bInferenceChunkerOptions
	sessionID string
	// we keep X bytes in memory before emitting an event for the stream
	bufferStream string
	// the entire response for the session is kept in memory
	// so we can submit a complete result when we are complete with a single session
	bufferSession string
	// this means "have we seen the [/INST] so are now into the answer?"
	active       bool
	eventHandler WorkerEventHandler
}

func newMistral7bInferenceChunker(eventHandler WorkerEventHandler, options mistral7bInferenceChunkerOptions) *mistral7bInferenceChunker {
	return &mistral7bInferenceChunker{
		options:       options,
		sessionID:     "",
		bufferStream:  "",
		bufferSession: "",
		active:        false,
		eventHandler:  eventHandler,
	}
}

func (chunker *mistral7bInferenceChunker) addBuffer(word string) {
	chunker.bufferStream += word + " "
	chunker.bufferSession += word + " "
	if len(chunker.bufferStream) > chunker.options.bufferSize {
		chunker.emitStream()
	}
}

func (chunker *mistral7bInferenceChunker) emitStream() {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeStream,
		SessionID: chunker.sessionID,
		Message:   chunker.bufferStream,
	})
	chunker.bufferStream = ""
}

func (chunker *mistral7bInferenceChunker) emitStreamDone() {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeStream,
		SessionID: chunker.sessionID,
		Message:   "",
		Done:      true,
	})
}

func (chunker *mistral7bInferenceChunker) emitResult() {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Message:   chunker.bufferSession,
	})
	chunker.bufferSession = ""
}

func (chunker *mistral7bInferenceChunker) write(word string) error {
	log.Info().Msgf("👉 '%s' 👈", strings.Replace(word, "\n", "\\n", -1))
	// [SESSION_START]session_id=7d11a9ef-a192-426c-bc8e-6bd2c6364b46
	if strings.HasPrefix(word, "[SESSION_START]") {
		log.Info().Msg("👉 case 1")
		parts := strings.Split(word, "=")
		if len(parts) < 2 {
			// we reset here because we got a session start line with no ID
			// which is very strange
			chunker.reset()
			return fmt.Errorf("invalid session start line: %s", word)
		}
		chunker.sessionID = parts[1]
		chunker.active = true
	} else if strings.HasPrefix(word, "[SESSION_END]") {
		log.Info().Msg("👉 case 2")
		// Signal that we are done with this session for
		// any streaming clients
		chunker.emitStreamDone()

		chunker.emitResult()

		// Reset the buffer
		chunker.reset()
	} else if chunker.sessionID != "" {
		log.Info().Msg("👉 case 3")
		if chunker.active {
			if strings.HasSuffix(word, "</s>\n") {
				word = strings.Replace(word, "</s>", "", 1)
			}
			log.Info().Msg("👉 case 4")
			chunker.addBuffer(word)
		}
	}
	return nil
}

func (chunker *mistral7bInferenceChunker) reset() {
	chunker.sessionID = ""
	chunker.bufferStream = ""
	chunker.bufferSession = ""
	chunker.active = false
}

type mistral7bFinetuneChunkerOptions struct {
	// if defined - we must wait until we see this word
	// before we start to activate percentages
	// this is because the fine tuning emits percentages
	// before the actual training starts so causes
	// the loading bar to flicker back and forth
	progressActivationWord string
}

type mistral7bFinetuneChunker struct {
	sessionID      string
	progressActive bool
	options        mistral7bFinetuneChunkerOptions
	eventHandler   WorkerEventHandler
}

func newMistral7bFinetuneChunker(eventHandler WorkerEventHandler, options mistral7bFinetuneChunkerOptions) *mistral7bFinetuneChunker {
	return &mistral7bFinetuneChunker{
		sessionID:      "",
		eventHandler:   eventHandler,
		options:        options,
		progressActive: false,
	}
}

func (chunker *mistral7bFinetuneChunker) write(word string) error {
	// [SESSION_START]session_id=7d11a9ef-a192-426c-bc8e-6bd2c6364b46
	if strings.HasPrefix(word, "[SESSION_START]") {
		parts := strings.Split(word, "=")
		if len(parts) < 2 {
			// we reset here because we got a session start line with no ID
			// which is very strange
			return fmt.Errorf("invalid session start line: %s", word)
		}
		chunker.sessionID = parts[1]
	} else if strings.HasPrefix(word, "[SESSION_END_LORA_DIR]") {
		// e.g. [SESSION_END_LORA_DIR]lora_dir=/tmp/helix/results/123
		parts := strings.Split(word, "=")
		if len(parts) < 2 {
			// we reset here because we got a session start line with no ID
			// which is very strange
			return fmt.Errorf("invalid session start line: %s", word)
		}
		chunker.eventHandler(&types.RunnerTaskResponse{
			Type:      types.WorkerTaskResponseTypeResult,
			SessionID: chunker.sessionID,
			LoraDir:   parts[1],
			Files:     []string{},
		})
		chunker.reset()
	} else if chunker.sessionID != "" {
		if chunker.options.progressActivationWord != "" && !chunker.progressActive && strings.HasPrefix(word, chunker.options.progressActivationWord) {
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
			chunker.eventHandler(&types.RunnerTaskResponse{
				Type:      types.WorkerTaskResponseTypeProgress,
				SessionID: chunker.sessionID,
				Progress:  progress,
			})
		}
	}
	return nil
}

func (chunker *mistral7bFinetuneChunker) reset() {
	chunker.sessionID = ""
	chunker.progressActive = false
}

// Compile-time interface check:
var _ Model = (*Mistral7bInstruct01)(nil)
