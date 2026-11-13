// Package exa is a websearch.Provider that forwards every call to the Exa
// API unchanged. It is the reference implementation that defines parity.
package exa

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/websearch"
)

// Answer and deep search can take over a minute; callers bound individual
// requests with their context.
const clientTimeout = 5 * time.Minute

// Upstream error bodies are read only to build an error message.
const maxErrorBody = 64 << 10

type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

var _ websearch.Provider = (*Provider)(nil)

func New(apiKey, baseURL string) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("exa: API key is required")
	}
	if baseURL == "" {
		return nil, errors.New("exa: base URL is required")
	}
	return &Provider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: clientTimeout},
	}, nil
}

func (p *Provider) Search(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
	var resp websearch.SearchResponse
	if err := p.call(ctx, "/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (p *Provider) Contents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	var resp websearch.ContentsResponse
	if err := p.call(ctx, "/contents", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (p *Provider) FindSimilar(ctx context.Context, req *websearch.FindSimilarRequest) (*websearch.FindSimilarResponse, error) {
	var resp websearch.FindSimilarResponse
	if err := p.call(ctx, "/findSimilar", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (p *Provider) Answer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	var resp websearch.AnswerResponse
	if err := p.call(ctx, "/answer", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (p *Provider) StreamSearch(ctx context.Context, req *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
	r := *req
	r.Stream = true
	return p.stream(ctx, "/search", &r, func(data []byte) error {
		var ev websearch.SearchEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return upstreamError("decode search event: %v", err)
		}
		return emit(ev)
	})
}

func (p *Provider) StreamAnswer(ctx context.Context, req *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
	r := *req
	r.Stream = true
	return p.stream(ctx, "/answer", &r, func(data []byte) error {
		var ev websearch.AnswerEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return upstreamError("decode answer event: %v", err)
		}
		return emit(ev)
	})
}

func (p *Provider) call(ctx context.Context, path string, body, out any) error {
	resp, err := p.post(ctx, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return upstreamError("decode %s response: %v", path, err)
	}
	return nil
}

// stream posts body and calls handle with the data of every SSE event until
// the [DONE] terminator or the end of the body.
func (p *Provider) stream(ctx context.Context, path string, body any, handle func([]byte) error) error {
	resp, err := p.post(ctx, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = readSSE(resp.Body, func(data []byte) error {
		if err := streamError(data); err != nil {
			return err
		}
		return handle(data)
	})
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// post sends body and returns the response when it is a 2xx; otherwise the
// upstream error as *websearch.Error.
func (p *Provider) post(ctx context.Context, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("exa: encode %s request: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("exa: build %s request: %w", path, err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, upstreamError("exa %s: %v", path, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, decodeError(resp)
}

// errorEnvelope is Exa's {requestId, error, tag} error body.
type errorEnvelope struct {
	RequestID string `json:"requestId"`
	Error     string `json:"error"`
	Tag       string `json:"tag"`
}

func decodeError(resp *http.Response) *websearch.Error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil {
		return &websearch.Error{Status: resp.StatusCode, Tag: websearch.TagUpstreamError,
			Message: fmt.Sprintf("exa returned %d; reading body: %v", resp.StatusCode, err)}
	}
	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil && env.Error != "" {
		tag := env.Tag
		if tag == "" {
			tag = websearch.TagUpstreamError
		}
		return &websearch.Error{Status: resp.StatusCode, Tag: tag, Message: env.Error}
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &websearch.Error{Status: resp.StatusCode, Tag: websearch.TagUpstreamError,
		Message: fmt.Sprintf("exa returned %d: %s", resp.StatusCode, msg)}
}

// streamErrorEvent is the error frame Exa sends inside an SSE stream:
// {"tag":"ERROR","payload":{"error":{"code":..,"message":..}}}.
type streamErrorEvent struct {
	Tag     string `json:"tag"`
	Payload struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	} `json:"payload"`
}

func streamError(data []byte) error {
	var ev streamErrorEvent
	if json.Unmarshal(data, &ev) != nil || ev.Tag != "ERROR" {
		return nil
	}
	status := http.StatusBadGateway
	var code int
	if json.Unmarshal(ev.Payload.Error.Code, &code) == nil && code >= 400 && code < 600 {
		status = code
	}
	msg := ev.Payload.Error.Message
	if msg == "" {
		msg = "exa stream error"
	}
	return &websearch.Error{Status: status, Tag: websearch.TagUpstreamError, Message: msg}
}

// readSSE calls handle with the data of each event. Multi-line data fields
// are joined with "\n"; other fields and comments are ignored.
func readSSE(r io.Reader, handle func([]byte) error) error {
	br := bufio.NewReader(r)
	var data []byte
	hasData := false
	dispatch := func() (bool, error) {
		if !hasData {
			return false, nil
		}
		d := data
		data, hasData = nil, false
		if string(bytes.TrimSpace(d)) == "[DONE]" {
			return true, nil
		}
		return false, handle(d)
	}
	for {
		line, err := br.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return upstreamError("read stream: %v", err)
		}
		eof := errors.Is(err, io.EOF)
		line = bytes.TrimRight(line, "\r\n")

		if len(line) == 0 {
			done, herr := dispatch()
			if herr != nil || done {
				return herr
			}
		} else if v, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			v = bytes.TrimPrefix(v, []byte(" "))
			if hasData {
				data = append(data, '\n')
			}
			data = append(data, v...)
			hasData = true
		}

		if eof {
			_, herr := dispatch()
			return herr
		}
	}
}

func upstreamError(format string, args ...any) *websearch.Error {
	return &websearch.Error{Status: http.StatusBadGateway, Tag: websearch.TagUpstreamError, Message: fmt.Sprintf(format, args...)}
}
