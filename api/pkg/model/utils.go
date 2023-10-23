package model

import (
	"bytes"
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
