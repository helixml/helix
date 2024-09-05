package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	openai "github.com/lukemarsden/go-openai2"
)

// NewOpenAIStreamingAdapter returns a new OpenAI streaming adapter which allows
// to write into the io.Writer and read from the stream directly
func NewOpenAIStreamingAdapter(req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, *io.PipeWriter, error) {
	pr, pw := io.Pipe()

	ht := &helixTransport{
		reader: pr,
		writer: pw,
	}

	// Create a fake HTTP client with a custom transport that will be feeding the stream
	config := openai.DefaultConfig("helix")
	config.HTTPClient = &http.Client{
		Transport: ht,
	}

	client := openai.NewClientWithConfig(config)

	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating chat completion stream: %w", err)
	}

	return stream, pw, nil
}

type helixTransport struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (t *helixTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	readCloser := io.NopCloser(t.reader)

	return &http.Response{
		StatusCode: 200,
		Body:       readCloser,
	}, nil
}

func WriteChatCompletionStream(w io.Writer, chunk *openai.ChatCompletionStreamResponse) error {
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
