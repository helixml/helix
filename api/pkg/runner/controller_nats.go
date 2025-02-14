package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type NatsControllerConfig struct {
	RunnerID  string
	PS        pubsub.PubSub
	ServerURL string
}

type NatsController struct {
	pubsub    pubsub.PubSub
	serverURL string
}

func NewNatsController(ctx context.Context, config *NatsControllerConfig) (*NatsController, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Parse the server URL to make sure it's valid
	parsedURL, err := url.Parse(config.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	controller := &NatsController{
		pubsub:    config.PS,
		serverURL: parsedURL.Scheme + "://" + parsedURL.Host,
	}

	// Monitor connection status
	config.PS.OnConnectionStatus(func(status pubsub.ConnectionStatus) {
		switch status {
		case pubsub.Connected:
			log.Info().Str("runner_id", config.RunnerID).Msg("nats connection established")
		case pubsub.Disconnected:
			log.Warn().Str("runner_id", config.RunnerID).Msg("nats connection lost")
		case pubsub.Reconnecting:
			log.Info().Str("runner_id", config.RunnerID).Msg("nats connection reconnecting")
		}
	})

	// Initial subscription setup
	if err := controller.setupSubscription(ctx, config.RunnerID); err != nil {
		return nil, err
	}

	// goroutine to periodically ping the scheduler
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				if err := controller.pubsub.Publish(ctx, pubsub.GetRunnerConnectedQueue(config.RunnerID), []byte("ping")); err != nil {
					log.Error().Err(err).Msg("failed to publish runner ping")
				}
			}
		}
	}()

	return controller, nil
}

// setupSubscription handles the subscription and connection announcement
func (c *NatsController) setupSubscription(ctx context.Context, runnerID string) error {
	log.Debug().Str("runner_id", runnerID).Str("queue", pubsub.GetRunnerQueue(runnerID)).Msg("subscribing to NATS queue")

	subscription, err := c.pubsub.SubscribeWithCtx(ctx, pubsub.GetRunnerQueue(runnerID), c.handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	go func() {
		<-ctx.Done()
		err := subscription.Unsubscribe()
		if err != nil {
			log.Error().Err(err).Msg("error unsubscribing from runner queue")
		}
	}()

	log.Debug().Str("runner_id", runnerID).Str("queue", pubsub.GetRunnerConnectedQueue(runnerID)).Msg("publishing to runner.connected queue")
	if err := c.pubsub.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected")); err != nil {
		return fmt.Errorf("failed to publish connected message: %w", err)
	}

	return nil
}

func (c *NatsController) handler(ctx context.Context, msg *nats.Msg) error {
	// Create a new context that we can cancel if needed
	taskCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()
		var req types.Request
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Error().Err(err).Msg("failed to unmarshal request")
			c.respondWithError(msg, err)
			return
		}

		// Execute the task via an HTTP handler
		response := c.executeTaskViaHTTP(taskCtx, msg.Header, req)

		responseBytes, err := json.Marshal(response)
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal response")
			c.respondWithError(msg, err)
			return
		}
		err = msg.Respond(responseBytes)
		if err != nil {
			log.Error().Err(err).Msg("failed to respond to nats")
			c.respondWithError(msg, err)
			return
		}
	}()

	return nil
}

func (c *NatsController) executeTaskViaHTTP(ctx context.Context, headers nats.Header, task types.Request) *types.Response {
	// Route based on request type
	if headers.Get(pubsub.HelixNatsReplyHeader) != "" {
		return c.handleNatsReplyRequest(ctx, task, headers.Get(pubsub.HelixNatsReplyHeader))
	}

	return c.handleGenericHTTPRequest(ctx, task)
}

// handleGenericHTTPRequest processes standard HTTP requests
func (c *NatsController) handleGenericHTTPRequest(ctx context.Context, task types.Request) *types.Response {
	url, err := url.Parse(task.URL)
	if err != nil {
		log.Error().Err(err).Str("url", task.URL).Msg("failed to parse URL")
		return &types.Response{StatusCode: 400, Body: []byte("Unable to parse request URL")}
	}

	req, err := http.NewRequestWithContext(ctx, task.Method, c.serverURL+url.Path, bytes.NewBuffer(task.Body))
	if err != nil {
		log.Error().Err(err).Msg("failed to create HTTP request")
		return &types.Response{StatusCode: 500, Body: []byte("Failed to create request")}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("failed to execute HTTP request")
		return &types.Response{StatusCode: 500, Body: []byte("Request failed")}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read response body")
		return &types.Response{StatusCode: 500, Body: []byte("Failed to read response")}
	}

	return &types.Response{
		StatusCode: resp.StatusCode,
		Body:       body,
	}
}

// handleNatsReplyRequest processes nats reply requests
func (c *NatsController) handleNatsReplyRequest(ctx context.Context, task types.Request, responseQueue string) *types.Response {
	url, err := url.Parse(task.URL)
	if err != nil {
		log.Error().Err(err).Str("url", task.URL).Msg("failed to parse URL")
		return &types.Response{StatusCode: 400, Body: []byte("Unable to parse request URL")}
	}

	var req types.RunnerNatsReplyRequest
	if err := json.Unmarshal(task.Body, &req); err != nil {
		log.Error().Err(err).Msg("failed to parse nats reply request")
		return &types.Response{StatusCode: 400, Body: []byte("Invalid nats reply request format")}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, task.Method, c.serverURL+url.Path, bytes.NewReader(req.Request))
	if err != nil {
		log.Error().Err(err).Msg("failed to create nats reply HTTP request")
		return &types.Response{StatusCode: 500, Body: []byte("Failed to create request")}
	}

	// Add session ID header has a hacky workaround to allow downstream APIs to use the session ID
	// and interaction ID
	httpReq.Header.Set(types.SessionIDHeader, req.SessionID)
	httpReq.Header.Set(types.InteractionIDHeader, req.InteractionID)

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("failed to execute nats reply HTTP request")
		return &types.Response{StatusCode: 500, Body: []byte("Request failed")}
	}
	defer resp.Body.Close()

	// Get response queue for publishing results
	start := time.Now()

	log.Trace().
		Str("content_type", resp.Header.Get("Content-Type")).
		Str("response_queue", responseQueue).
		Msg("received nats reply response")

	// Handle streaming vs non-streaming responses
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		log.Debug().Msg("handling streaming nats reply response")
		return c.handleStreamingResponse(ctx, req, resp, responseQueue, start)
	}
	log.Debug().Msg("handling regular nats reply response")
	return c.handleRegularResponse(ctx, req, resp, responseQueue, start)
}

func (c *NatsController) handleStreamingResponse(ctx context.Context, req types.RunnerNatsReplyRequest, resp *http.Response, responseQueue string, start time.Time) *types.Response {
	log.Debug().
		Str("response_queue", responseQueue).
		Msg("starting stream processing")

	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("stream processing cancelled")
			return &types.Response{StatusCode: 500, Body: []byte("Stream closed")}
		default:
			chunk, err := reader.ReadBytes('\n')
			if err == io.EOF {
				log.Debug().Msg("stream completed")
				return &types.Response{StatusCode: 200, Body: []byte("Stream completed")}
			}
			if err != nil {
				log.Error().Err(err).Msg("error reading stream chunk")
				return &types.Response{StatusCode: 500, Body: []byte("Stream error")}
			}

			// Skip empty chunks
			if len(bytes.TrimSpace(chunk)) == 0 {
				log.Trace().Msg("skipping empty chunk")
				continue
			}

			log.Trace().
				Int("chunk_size", len(chunk)).
				Str("chunk", string(chunk)).
				Msg("processing stream chunk")

			if err := c.publishResponse(ctx, responseQueue, req, chunk, start); err != nil {
				log.Error().Err(err).Msg("failed to publish response")
			}
		}
	}
}

func (c *NatsController) handleRegularResponse(ctx context.Context, req types.RunnerNatsReplyRequest, resp *http.Response, responseQueue string, start time.Time) *types.Response {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read response body")
		return &types.Response{StatusCode: 500, Body: []byte("Failed to read response")}
	}

	log.Trace().
		Str("request_id", req.RequestID).
		Str("response_queue", responseQueue).
		Str("response", string(body)).
		Msg("handling regular nats reply response")

	if err := c.publishResponse(ctx, responseQueue, req, body, start); err != nil {
		log.Error().Err(err).Msg("failed to publish response")
		return &types.Response{StatusCode: 500, Body: []byte("Failed to publish response")}
	}

	return &types.Response{
		StatusCode: resp.StatusCode,
		Body:       body,
	}
}

// publishResponse publishes responses to NATS
func (c *NatsController) publishResponse(ctx context.Context, queue string, req types.RunnerNatsReplyRequest, resp []byte, start time.Time) error {
	log.Trace().
		Str("queue", queue).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("publishing nats reply response")

	infResponse := &types.RunnerNatsReplyResponse{
		RequestID:     req.RequestID,
		CreatedAt:     time.Now(),
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		DurationMs:    time.Since(start).Milliseconds(),
		Response:      resp,
	}

	respData, err := json.Marshal(infResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := c.pubsub.Publish(ctx, queue, respData); err != nil {
		log.Error().
			Err(err).
			Str("queue", queue).
			Int("response_size", len(respData)).
			Msg("failed to publish to queue")
		return fmt.Errorf("failed to publish response: %w", err)
	}

	return nil
}

// Helper method to handle error responses
func (c *NatsController) respondWithError(msg *nats.Msg, err error) {
	response := &types.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       []byte(err.Error()),
	}

	responseBytes, _ := json.Marshal(response)
	if err := msg.Respond(responseBytes); err != nil {
		log.Error().Err(err).Msg("failed to send error response")
	}
}
