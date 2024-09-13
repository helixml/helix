package system

import (
	"bytes"
	"testing"
)

// This test previously used to panic when you passed enough new data that exceeded the buffer's limit.
func TestLimitedBufferPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Received panic: %v", r)
		}
	}()

	// Create a LimitedBuffer with a small limit
	limit := 10
	buf := NewLimitedBuffer(limit)

	// Write data to the buffer in multiple steps
	data1 := bytes.Repeat([]byte("a"), 5)  // 5 bytes
	data2 := bytes.Repeat([]byte("b"), 20) // More than the limit

	// This should cause a panic in the old code
	buf.Write(data1)
	buf.Write(data2)
}
