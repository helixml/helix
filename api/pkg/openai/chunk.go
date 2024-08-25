package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	openai "github.com/lukemarsden/go-openai2"
)

func WriteStreamingResponseChunk(w io.Writer, chunk *openai.ChatCompletionStreamResponse) error {
	bts, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("error marshalling chunk: %w", err)
	}

	return WriteChunk(w, bts)
}

func WriteChunk(w io.Writer, chunk []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(chunk))
	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
	}

	// Flush the ResponseWriter buffer to send the chunk immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}
