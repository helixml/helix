package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/websearch"
)

// webSearchTimeout bounds a web search call whose context has no deadline:
// deep search and answers fetch and read many pages.
const webSearchTimeout = 5 * time.Minute

func (c *HelixClient) WebSearch(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
	var resp websearch.SearchResponse
	if err := c.postWebSearch(ctx, "/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HelixClient) WebSearchContents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	var resp websearch.ContentsResponse
	if err := c.postWebSearch(ctx, "/contents", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HelixClient) WebSearchFindSimilar(ctx context.Context, req *websearch.FindSimilarRequest) (*websearch.FindSimilarResponse, error) {
	var resp websearch.FindSimilarResponse
	if err := c.postWebSearch(ctx, "/findSimilar", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HelixClient) WebSearchAnswer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	var resp websearch.AnswerResponse
	if err := c.postWebSearch(ctx, "/answer", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HelixClient) postWebSearch(ctx context.Context, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	ctx, cancel := webSearchContext(ctx)
	defer cancel()
	return c.makeRequest(ctx, http.MethodPost, "/websearch"+path, bytes.NewReader(body), resp)
}

// StreamWebSearchAnswer streams /answer, calling onEvent for every event
// until the server sends [DONE].
func (c *HelixClient) StreamWebSearchAnswer(ctx context.Context, req *websearch.AnswerRequest, onEvent func(websearch.AnswerEvent) error) error {
	streamReq := *req
	streamReq.Stream = true
	body, err := json.Marshal(&streamReq)
	if err != nil {
		return err
	}
	ctx, cancel := webSearchContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/websearch/answer", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status code %d (%s)", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			return nil
		}
		var event streamedAnswerEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("decode answer event: %w", err)
		}
		if event.Tag == websearch.AnswerErrorTag {
			return fmt.Errorf("answer failed (%d): %s", event.Payload.Error.Code, event.Payload.Error.Message)
		}
		if err := onEvent(event.AnswerEvent); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("answer stream ended without [DONE]")
}

// streamedAnswerEvent is an /answer stream event: an AnswerEvent or, when
// the answer fails mid-stream, a websearch.AnswerErrorEvent.
type streamedAnswerEvent struct {
	websearch.AnswerEvent
	Tag     string                       `json:"tag"`
	Payload websearch.AnswerErrorPayload `json:"payload"`
}

func webSearchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, webSearchTimeout)
}
