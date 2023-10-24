package model

import (
	"bytes"

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

func getUserInteraction(session *types.Session) (*types.Interaction, error) {
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		interaction := session.Interactions[i]
		if interaction.Creator == types.CreatorTypeUser {
			return &interaction, nil
		}
	}
	return nil, nil
}
