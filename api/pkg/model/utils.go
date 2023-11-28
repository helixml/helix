package model

import (
	"fmt"
	"path"
	"time"
	"unicode/utf8"

	"github.com/lukemarsden/helix/api/pkg/types"
)

// define 1 GB as a uint64 number of bytes
const GB uint64 = 1024 * 1024 * 1024
const MB uint64 = 1024 * 1024

// get the most recent user interaction
func GetUserInteraction(session *types.Session) (*types.Interaction, error) {
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		interaction := session.Interactions[i]
		if interaction.Creator == types.CreatorTypeUser {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no user interaction found")
}

func GetSystemInteraction(session *types.Session) (*types.Interaction, error) {
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		interaction := session.Interactions[i]
		if interaction.Creator == types.CreatorTypeSystem {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no system interaction found")
}

// update the most recent system interaction

type InteractionUpdater func(*types.Interaction) (*types.Interaction, error)

func UpdateSystemInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	targetInteraction.Updated = time.Now()
	updatedInteraction, err := updater(targetInteraction)
	if err != nil {
		return nil, err
	}

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == targetInteraction.ID {
			newInteractions = append(newInteractions, *updatedInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions
	session.Updated = time.Now()

	return session, nil
}

func GetSessionSummary(session *types.Session) (*types.SessionSummary, error) {
	systemInteraction, err := GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}
	userInteraction, err := GetUserInteraction(session)
	if err != nil {
		return nil, err
	}
	summary := ""
	if session.Mode == types.SessionModeInference {
		summary = userInteraction.Message
	} else if session.Mode == types.SessionModeFinetune {
		summary = fmt.Sprintf("fine tuning on %d files", len(userInteraction.Files))
	} else {
		return nil, fmt.Errorf("invalid session mode")
	}
	return &types.SessionSummary{
		SessionID:     session.ID,
		Name:          session.Name,
		InteractionID: systemInteraction.ID,
		Mode:          session.Mode,
		Type:          session.Type,
		ModelName:     session.ModelName,
		Owner:         session.Owner,
		LoraDir:       session.LoraDir,
		Created:       systemInteraction.Created,
		Updated:       systemInteraction.Updated,
		Scheduled:     systemInteraction.Scheduled,
		Completed:     systemInteraction.Completed,
		Summary:       summary,
	}, nil
}

// each model get's to decide what it's task looks like
// but this is the vanilla "most models return this"
// version - models call this and are free to override fields
func getGenericTask(session *types.Session) (*types.RunnerTask, error) {
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
		return &types.RunnerTask{
			Prompt:  lastInteraction.Message,
			LoraDir: session.LoraDir,
		}, nil
	} else if session.Mode == types.SessionModeFinetune {
		if len(lastInteraction.Files) == 0 {
			return nil, fmt.Errorf("session has no files")
		}
		// we expect all of the files to have been downloaded
		// by the controller and put into a shared folder
		// so - we extract the folder path from the first file
		// and pass it into the python job as the input dir
		return &types.RunnerTask{
			DatasetDir: path.Dir(lastInteraction.Files[0]),
		}, nil
	} else {
		return nil, fmt.Errorf("invalid session mode")
	}
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
