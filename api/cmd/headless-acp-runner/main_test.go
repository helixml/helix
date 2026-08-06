package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type wsFixture struct {
	client *websocket.Conn
	events chan syncMessage
}

type nopWriteCloser struct {
	bytes.Buffer
}

func (*nopWriteCloser) Close() error { return nil }

func newWSFixture(t *testing.T) *wsFixture {
	t.Helper()
	events := make(chan syncMessage, 16)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for {
			var event syncMessage
			if conn.ReadJSON(&event) != nil {
				return
			}
			events <- event
		}
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return &wsFixture{client: conn, events: events}
}

func fakeACP(t *testing.T, updates []rpcMessage, holdResponse bool) (*acpClient, chan struct{}) {
	t.Helper()
	reader, writer := ioPipe(t)
	c := &acpClient{stdin: writer, pending: map[string]chan rpcMessage{}, updates: make(chan rpcMessage, 16), done: make(chan struct{})}
	release := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(reader)
		var request rpcMessage
		for scanner.Scan() {
			_ = json.Unmarshal(scanner.Bytes(), &request)
			if len(request.ID) > 0 && request.Method == "session/prompt" {
				break
			}
		}
		if len(request.ID) == 0 {
			return
		}
		for _, update := range updates {
			c.updates <- update
		}
		if holdResponse {
			<-release
		}
		c.mu.Lock()
		pending := c.pending[string(request.ID)]
		delete(c.pending, string(request.ID))
		c.mu.Unlock()
		pending <- rpcMessage{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{"stopReason":"end_turn"}`)}
	}()
	return c, release
}

func ioPipe(t *testing.T) (*io.PipeReader, *io.PipeWriter) {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() { reader.Close(); writer.Close() })
	return reader, writer
}

func textUpdate(text string) rpcMessage {
	params, _ := json.Marshal(map[string]any{"update": map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content":       map[string]string{"type": "text", "text": text},
	}})
	return rpcMessage{JSONRPC: "2.0", Method: "session/update", Params: params}
}

func TestRunPromptOrdersCumulativeChunksBeforeCompletion(t *testing.T) {
	fixture := newWSFixture(t)
	acp, _ := fakeACP(t, []rpcMessage{textUpdate("hello "), textUpdate("world")}, false)
	commands := make(chan command)
	err := runPrompt(context.Background(), acp, fixture.client, &sync.Mutex{}, "ses_1", "acp_1", command{
		Type: "chat_message", Data: map[string]any{"request_id": "req_1", "message": "hi"},
	}, commands)
	require.NoError(t, err)

	var events []syncMessage
	for len(events) < 4 {
		select {
		case event := <-fixture.events:
			events = append(events, event)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for sync events")
		}
	}
	require.Equal(t, []string{"thread_created", "message_added", "message_added", "message_completed"}, []string{
		events[0].EventType, events[1].EventType, events[2].EventType, events[3].EventType,
	})
	require.Equal(t, "acp-req_1", events[1].Data["message_id"])
	require.Equal(t, "hello ", events[1].Data["content"])
	require.Equal(t, "hello world", events[2].Data["content"])
}

func TestRunPromptCancellationIsRequestScopedAndDoesNotComplete(t *testing.T) {
	fixture := newWSFixture(t)
	acp, release := fakeACP(t, nil, true)
	commands := make(chan command, 2)
	commands <- command{Type: "cancel_current_turn", Data: map[string]any{"request_id": "other"}}
	commands <- command{Type: "cancel_current_turn", Data: map[string]any{"request_id": "req_1"}}
	time.AfterFunc(10*time.Millisecond, func() { close(release) })
	err := runPrompt(context.Background(), acp, fixture.client, &sync.Mutex{}, "ses_1", "acp_1", command{
		Type: "chat_message", Data: map[string]any{"request_id": "req_1", "message": "hi", "acp_thread_id": "acp_1"},
	}, commands)
	require.NoError(t, err)

	event := <-fixture.events
	require.Equal(t, "turn_cancelled", event.EventType)
	require.Equal(t, "cancelled", event.Data["status"])
	select {
	case event := <-fixture.events:
		require.NotEqual(t, "message_completed", event.EventType)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestACPCallReturnsWhenProcessCloses(t *testing.T) {
	done := make(chan struct{})
	close(done)
	acp := &acpClient{
		stdin:   &nopWriteCloser{},
		pending: map[string]chan rpcMessage{},
		updates: make(chan rpcMessage),
		done:    done,
	}
	require.EqualError(t, acp.call(context.Background(), "initialize", map[string]any{}, nil), "ACP process closed")
}

func TestRunPromptReturnsWhenHelixConnectionCloses(t *testing.T) {
	fixture := newWSFixture(t)
	acp, release := fakeACP(t, nil, true)
	commands := make(chan command)
	close(commands)
	err := runPrompt(context.Background(), acp, fixture.client, &sync.Mutex{}, "ses_1", "acp_1", command{
		Type: "chat_message", Data: map[string]any{"request_id": "req_1", "message": "hi", "acp_thread_id": "acp_1"},
	}, commands)
	close(release)
	require.EqualError(t, err, "Helix connection closed")
}

func TestApplyGooseConfig(t *testing.T) {
	t.Setenv("GOOSE_PROVIDER", "")
	t.Setenv("GOOSE_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	applyGooseConfig("http://api:8080", codeAgentConfig{
		Model: "claude-sonnet", BaseURL: "http://localhost:8080/v1", APIType: "anthropic",
	})
	require.Equal(t, "anthropic", os.Getenv("GOOSE_PROVIDER"))
	require.Equal(t, "claude-sonnet", os.Getenv("GOOSE_MODEL"))
	require.Equal(t, "http://api:8080/v1", os.Getenv("ANTHROPIC_BASE_URL"))
}

func TestACPMCPServers(t *testing.T) {
	servers := acpMCPServers(map[string]any{
		"shell": map[string]any{"command": "tool", "args": []any{"serve"}, "env": map[string]any{"TOKEN": "secret"}},
		"org":   map[string]any{"url": "http://api/mcp", "headers": map[string]any{"Authorization": "Bearer key"}},
	})
	require.Equal(t, []map[string]any{
		{"name": "org", "type": "http", "url": "http://api/mcp", "headers": []map[string]string{{"name": "Authorization", "value": "Bearer key"}}},
		{"name": "shell", "command": "tool", "args": []string{"serve"}, "env": []map[string]string{{"name": "TOKEN", "value": "secret"}}},
	}, servers)
}

func TestPrepareACPLoadsPersistedSessionWithMCPServers(t *testing.T) {
	requestReader, requestWriter := ioPipe(t)
	responseReader, responseWriter := ioPipe(t)
	client := &acpClient{stdin: requestWriter, pending: map[string]chan rpcMessage{}, updates: make(chan rpcMessage, 4)}
	go client.readLoop(responseReader)
	methods := make(chan string, 2)
	go func() {
		scanner := bufio.NewScanner(requestReader)
		for scanner.Scan() {
			var request rpcMessage
			_ = json.Unmarshal(scanner.Bytes(), &request)
			methods <- request.Method
			var result any = map[string]any{}
			if request.Method == "initialize" {
				result = map[string]any{"agentCapabilities": map[string]bool{"loadSession": true}}
			}
			response, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			_, _ = responseWriter.Write(append(response, '\n'))
		}
	}()
	statePath := t.TempDir() + "/state.json"
	require.NoError(t, os.WriteFile(statePath, []byte(`{"acp_session_id":"acp_saved"}`), 0600))
	mcp := []map[string]any{{"type": "http", "name": "org", "url": "http://api/mcp"}}
	sessionID, err := prepareACP(context.Background(), client, "/workspace", statePath, mcp)
	require.NoError(t, err)
	require.Equal(t, "acp_saved", sessionID)
	require.Equal(t, "initialize", <-methods)
	require.Equal(t, "session/load", <-methods)
}
