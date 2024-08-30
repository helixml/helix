package transport

import (
	"context"
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
