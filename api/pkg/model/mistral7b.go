package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"unicode/utf8"

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

func (l *Mistral7bInstruct01) GetTask(session *types.Session) (*types.WorkerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}
	task.Prompt = fmt.Sprintf("[INST]%s[/INST]", task.Prompt)
	return task, nil
}

func (l *Mistral7bInstruct01) GetTextStream(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, error) {
	if mode == types.SessionModeInference {

		// this understands the context of each word and keeps state
		// to manage the session output window and emit events
		// via the event handler
		chunker := newMistral7bTextChunker(eventHandler, mistral7bTextChunkerOptions{
			bufferSize: 32,
		})

		// this will get called for each word
		// we have already replaced newlines with "[NEWLINE]"
		stream := NewTextStream(scanWordsPreserveNewlines, func(chunk string) {
			err := chunker.write(chunk)
			if err != nil {
				log.Error().Msgf("error writing word to chunker: %s", err)
			}
		})

		return stream, nil
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
		return nil, fmt.Errorf("invalid Mistral7bInstruct01 session mode: %s", sessionFilter.Mode)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cmd.Env = []string{
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join(wd, "..", "axolotl"))),
		fmt.Sprintf("HELIX_GET_JOB_URL=%s", config.TaskURL),
		fmt.Sprintf("HELIX_GET_SESSION_URL=%s", config.SessionURL),
		fmt.Sprintf("HELIX_RESPOND_JOB_URL=%s", config.ResponseURL),
	}

	return cmd, nil
}

type mistral7bTextChunkerOptions struct {
	// the max size of our buffer - we emit an event if the buffer get's bigger than this
	bufferSize int
}

type mistral7bTextChunker struct {
	options   mistral7bTextChunkerOptions
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

func newMistral7bTextChunker(eventHandler WorkerEventHandler, options mistral7bTextChunkerOptions) *mistral7bTextChunker {
	return &mistral7bTextChunker{
		options:       options,
		sessionID:     "",
		bufferStream:  "",
		bufferSession: "",
		active:        false,
		eventHandler:  eventHandler,
	}
}

func (chunker *mistral7bTextChunker) addBuffer(word string) {
	chunker.bufferStream += word + " "
	chunker.bufferSession += word + " "
	if len(chunker.bufferStream) > chunker.options.bufferSize {
		chunker.emitBufferStream()
	}
}

func (chunker *mistral7bTextChunker) emitBufferStream() {
	chunker.eventHandler(&types.WorkerTaskResponse{
		Type:      types.WorkerTaskResponseTypeStream,
		SessionID: chunker.sessionID,
		Message:   chunker.bufferStream,
	})
	chunker.bufferStream = ""
}

func (chunker *mistral7bTextChunker) emitBufferSession() {
	chunker.eventHandler(&types.WorkerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: chunker.sessionID,
		Message:   chunker.bufferSession,
	})
	chunker.bufferSession = ""
}

func (chunker *mistral7bTextChunker) write(word string) error {
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
		chunker.emitBufferSession()
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

func (chunker *mistral7bTextChunker) reset() {
	chunker.sessionID = ""
	chunker.bufferStream = ""
	chunker.bufferSession = ""
	chunker.active = false
}

// ////////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////////
// this is a copy of bufio.ScanWords from the go stdlib
// we want to preserve newlines in the output
// but we want to stream words to the client without waiting for a newline
// so - we can't use the stdlib bufio.ScanLines because otherwise we
// are waiting for a newline before we emit a word
// and we can't use bufio.ScanWords because it strips newlines before
// we get a chance to know they were there
//
// this implementation will replace newlines with "[NEWLINE]" sequence
func isSpace(r rune) bool {
	if r <= '\u00FF' {
		// Obvious ASCII ones: \t through \r plus space. Plus two Latin-1 oddballs.
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return true
		case '\u0085', '\u00A0':
			return true
		}
		return false
	}
	// High-valued ones.
	if '\u2000' <= r && r <= '\u200a' {
		return true
	}
	switch r {
	case '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000':
		return true
	}
	return false
}

func scanWordsPreserveNewlines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// Skip leading spaces except newlines.
	start := 0
	for width := 0; start < len(data); start += width {
		var r rune
		r, width = utf8.DecodeRune(data[start:])
		if !isSpace(r) || r == '\n' {
			break
		}
	}

	// Check for newline at the current position.
	if start < len(data) {
		r, _ := utf8.DecodeRune(data[start:])
		if r == '\n' {
			// Return "[NEWLINE]" token for newline character.
			return start + 1, []byte("\n"), nil
		}
	}

	// Scan until space, marking end of word.
	for width, i := 0, start; i < len(data); i += width {
		var r rune
		r, width = utf8.DecodeRune(data[i:])
		if isSpace(r) {
			return i + width, data[start:i], nil
		}
	}

	// If we're at EOF, we have a final, non-empty, non-terminated word. Return it.
	if atEOF && len(data) > start {
		return len(data), data[start:], nil
	}

	// Request more data.
	return start, nil, nil
}

// Compile-time interface check:
var _ Model = (*Mistral7bInstruct01)(nil)
