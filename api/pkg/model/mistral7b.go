package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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

func (l *Mistral7bInstruct01) GetTask(session *types.Session) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}
	task.Prompt = fmt.Sprintf("[INST]%s[/INST]", task.Prompt)
	return task, nil
}

func (l *Mistral7bInstruct01) GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error) {
	if mode == types.SessionModeInference {
		// this understands the context of each word and keeps state
		// to manage the session output window and emit events
		// via the event handler
		chunker := newMistral7bInferenceChunker(eventHandler, mistral7bInferenceChunkerOptions{
			bufferSize: 32,
		})

		// this will get called for each word
		// we have already replaced newlines with "[NEWLINE]"
		stdout := NewTextStream(scanWordsPreserveNewlines, func(chunk string) {
			err := chunker.write(chunk)
			if err != nil {
				log.Error().Msgf("error writing word to mistral inference chunker: %s", err)
			}
		})

		return stdout, nil, nil
	}

	return nil, nil, nil
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
		return nil, fmt.Errorf("invalid Mistral7bInstruct01 session mode: %s", sessionFilter.Mode)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
		fmt.Sprintf("HELIX_NEXT_TASK_URL=%s", config.NextTaskURL),
		fmt.Sprintf("HELIX_INITIAL_SESSION_URL=%s", config.InitialSessionURL),
	}

	return cmd, nil
}

type mistral7bInferenceChunkerOptions struct {
	// the max size of our buffer - we emit an event if the buffer get's bigger than this
	bufferSize int
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
func (chunker *mistral7bInferenceChunker) emitResult() {
	chunker.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Message:   chunker.bufferSession,
	})
	chunker.bufferSession = ""
}

func (chunker *mistral7bInferenceChunker) write(word string) error {
	// [SESSION_START]session_id=7d11a9ef-a192-426c-bc8e-6bd2c6364b46
	if strings.HasPrefix(word, "[SESSION_START]") {
		parts := strings.Split(word, "=")
		if len(parts) < 2 {
			// we reset here because we got a session start line with no ID
			// which is very strange
			chunker.reset()
			return fmt.Errorf("invalid session start line: %s", word)
		}
		chunker.sessionID = parts[1]
	} else if strings.HasPrefix(word, "[SESSION_END]") {
		chunker.emitResult()
		chunker.reset()
	} else if chunker.sessionID != "" {
		if chunker.active {
			if strings.HasSuffix(word, "</s>") {
				word = strings.Replace(word, "</s>", "", 1)
			}
			chunker.addBuffer(word)
		} else if strings.HasSuffix(word, "[/INST]") {
			chunker.active = true
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

// Compile-time interface check:
var _ Model = (*Mistral7bInstruct01)(nil)
