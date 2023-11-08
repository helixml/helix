package model

import (
	"bytes"
	"fmt"
	"path"

	"github.com/lukemarsden/helix/api/pkg/types"
)

// define 1 GB as a uint64 number of bytes
const GB uint64 = 1024 * 1024 * 1024

func splitOnSpace(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, ' '); i >= 0 {
		return i + 1, data[0:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

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

// each model get's to decide what it's task looks like
// but this is the vanilla "most models return this"
// version - models call this and are free to override fields
func getGenericTask(session *types.Session) (*types.WorkerTask, error) {
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
			Prompt:       lastInteraction.Message,
			FinetuneFile: session.FinetuneFile,
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
