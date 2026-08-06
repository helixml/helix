package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type acpClient struct {
	stdin   io.WriteCloser
	cmd     *exec.Cmd
	mu      sync.Mutex
	nextID  int
	pending map[string]chan rpcMessage
	updates chan rpcMessage
	done    chan struct{}
}

func startACP(ctx context.Context, command string) (*acpClient, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, errors.New("ACP command is empty")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &acpClient{stdin: stdin, cmd: cmd, pending: map[string]chan rpcMessage{}, updates: make(chan rpcMessage, 64), done: make(chan struct{})}
	go c.readLoop(stdout)
	return c, nil
}

func (c *acpClient) readLoop(r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for s.Scan() {
		var msg rpcMessage
		if json.Unmarshal(s.Bytes(), &msg) != nil {
			continue
		}
		if len(msg.ID) > 0 && msg.Method == "" {
			c.mu.Lock()
			ch := c.pending[string(msg.ID)]
			delete(c.pending, string(msg.ID))
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if len(msg.ID) > 0 && msg.Method == "session/request_permission" {
			go c.allowPermission(msg)
			continue
		}
		c.updates <- msg
	}
	close(c.updates)
	if c.done != nil {
		close(c.done)
	}
}

func (c *acpClient) write(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

func (c *acpClient) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	c.nextID++
	numericID := c.nextID
	id := strconv.Itoa(numericID)
	ch := make(chan rpcMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": numericID, "method": method, "params": params}); err != nil {
		c.deletePending(id)
		return err
	}
	select {
	case <-ctx.Done():
		c.deletePending(id)
		return ctx.Err()
	case <-c.done:
		c.deletePending(id)
		return errors.New("ACP process closed")
	case msg := <-ch:
		if msg.Error != nil {
			return fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(msg.Result, result)
		}
		return nil
	}
}

func (c *acpClient) deletePending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *acpClient) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *acpClient) allowPermission(msg rpcMessage) {
	var params struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	_ = json.Unmarshal(msg.Params, &params)
	var optionID string
	for _, option := range params.Options {
		if strings.HasPrefix(option.Kind, "allow") {
			optionID = option.OptionID
			break
		}
	}
	result := map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	if optionID != "" {
		result = map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
	}
	_ = c.write(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID), "result": result})
}

type sessionState struct {
	ACPSessionID string `json:"acp_session_id"`
}

type codeAgentConfig struct {
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
	APIType string `json:"api_type"`
}

func loadGooseConfig(ctx context.Context, apiURL, sessionID, token string) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiURL, "/")+"/api/v1/sessions/"+sessionID+"/zed-config", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.EqualFold(os.Getenv("ZED_HELIX_SKIP_TLS_VERIFY"), "true") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch code agent config: status %d", resp.StatusCode)
	}
	var body struct {
		CodeAgentConfig *codeAgentConfig `json:"code_agent_config"`
		ContextServers  map[string]any   `json:"context_servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.CodeAgentConfig == nil {
		return nil, errors.New("zed config has no code_agent_config")
	}
	applyGooseConfig(apiURL, *body.CodeAgentConfig)
	return acpMCPServers(body.ContextServers), nil
}

func acpMCPServers(contextServers map[string]any) []map[string]any {
	names := make([]string, 0, len(contextServers))
	for name := range contextServers {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]map[string]any, 0, len(names))
	for _, name := range names {
		config, ok := contextServers[name].(map[string]any)
		if !ok {
			continue
		}
		server := map[string]any{"name": name}
		if command, ok := config["command"].(string); ok && command != "" {
			server["command"] = command
			server["args"] = stringList(config["args"])
			server["env"] = nameValueList(config["env"])
		} else if serverURL, ok := config["url"].(string); ok && serverURL != "" {
			server["type"] = "http"
			server["url"] = serverURL
			server["headers"] = nameValueList(config["headers"])
		} else {
			continue
		}
		servers = append(servers, server)
	}
	return servers
}

func stringList(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func nameValueList(value any) []map[string]string {
	items, _ := value.(map[string]any)
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]string, 0, len(names))
	for _, name := range names {
		if text, ok := items[name].(string); ok {
			result = append(result, map[string]string{"name": name, "value": text})
		}
	}
	return result
}

func applyGooseConfig(apiURL string, config codeAgentConfig) {
	provider := "openai"
	baseEnv := "OPENAI_BASE_URL"
	if config.APIType == "anthropic" {
		provider = "anthropic"
		baseEnv = "ANTHROPIC_BASE_URL"
	}
	baseURL := config.BaseURL
	if parsedBase, err := url.Parse(baseURL); err == nil && parsedBase.Hostname() == "localhost" {
		if parsedAPI, parseErr := url.Parse(apiURL); parseErr == nil {
			parsedBase.Host = parsedAPI.Host
			baseURL = parsedBase.String()
		}
	}
	_ = os.Setenv("GOOSE_PROVIDER", provider)
	_ = os.Setenv("GOOSE_MODEL", config.Model)
	if baseURL != "" {
		_ = os.Setenv(baseEnv, baseURL)
	}
}

func prepareACP(ctx context.Context, c *acpClient, cwd, statePath string, mcpServers []map[string]any) (string, error) {
	var initialized struct {
		AgentCapabilities struct {
			LoadSession bool `json:"loadSession"`
		} `json:"agentCapabilities"`
	}
	err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]string{"name": "helix-headless-acp", "version": "0.1"},
	}, &initialized)
	if err != nil {
		return "", err
	}
	var state sessionState
	if data, readErr := os.ReadFile(statePath); readErr == nil && json.Unmarshal(data, &state) == nil && state.ACPSessionID != "" {
		if !initialized.AgentCapabilities.LoadSession {
			return "", errors.New("ACP agent cannot load the persisted session")
		}
		if err := c.call(ctx, "session/load", map[string]any{"sessionId": state.ACPSessionID, "cwd": cwd, "mcpServers": mcpServers}, nil); err != nil {
			return "", err
		}
		return state.ACPSessionID, nil
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": mcpServers}, &created); err != nil {
		return "", err
	}
	if created.SessionID == "" {
		return "", errors.New("ACP agent returned an empty session ID")
	}
	data, _ := json.Marshal(sessionState{ACPSessionID: created.SessionID})
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return "", err
	}
	return created.SessionID, nil
}

type syncMessage struct {
	SessionID string         `json:"session_id"`
	EventType string         `json:"event_type"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

type command struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

func sendEvent(conn *websocket.Conn, mu *sync.Mutex, sessionID, event string, data map[string]any) error {
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteJSON(syncMessage{SessionID: sessionID, EventType: event, Data: data, Timestamp: time.Now()})
}

func runPrompt(ctx context.Context, acp *acpClient, conn *websocket.Conn, writeMu *sync.Mutex, helixSessionID, acpSessionID string, cmd command, commands <-chan command) error {
	requestID, _ := cmd.Data["request_id"].(string)
	prompt, _ := cmd.Data["message"].(string)
	messageID := "acp-" + requestID
	if requestID == "" {
		requestID = messageID
	}
	incomingThreadID, _ := cmd.Data["acp_thread_id"].(string)
	if incomingThreadID == "" {
		if err := sendEvent(conn, writeMu, helixSessionID, "thread_created", map[string]any{"acp_thread_id": acpSessionID, "request_id": requestID}); err != nil {
			return err
		}
	} else if incomingThreadID != acpSessionID {
		return fmt.Errorf("incoming ACP thread %q does not match persisted thread %q", incomingThreadID, acpSessionID)
	}
	result := make(chan error, 1)
	go func() {
		result <- acp.call(ctx, "session/prompt", map[string]any{
			"sessionId": acpSessionID,
			"prompt":    []map[string]string{{"type": "text", "text": prompt}},
		}, nil)
	}()
	var cumulative string
	for {
		select {
		case err := <-result:
			if err != nil {
				return err
			}
			for {
				select {
				case update, ok := <-acp.updates:
					if !ok {
						return sendEvent(conn, writeMu, helixSessionID, "message_completed", map[string]any{"acp_thread_id": acpSessionID, "request_id": requestID, "agent_name": "goose"})
					}
					var updateErr error
					cumulative, updateErr = handleACPUpdate(conn, writeMu, helixSessionID, acpSessionID, requestID, messageID, cumulative, update)
					if updateErr != nil {
						return updateErr
					}
				default:
					return sendEvent(conn, writeMu, helixSessionID, "message_completed", map[string]any{"acp_thread_id": acpSessionID, "request_id": requestID, "agent_name": "goose"})
				}
			}
		case next, ok := <-commands:
			if !ok {
				return errors.New("Helix connection closed")
			}
			if next.Type != "cancel_current_turn" {
				continue
			}
			cancelRequest, _ := next.Data["request_id"].(string)
			if cancelRequest != "" && cancelRequest != requestID {
				continue
			}
			if err := acp.notify("session/cancel", map[string]any{"sessionId": acpSessionID}); err != nil {
				return err
			}
			if err := sendEvent(conn, writeMu, helixSessionID, "turn_cancelled", map[string]any{"acp_thread_id": acpSessionID, "request_id": requestID, "status": "cancelled"}); err != nil {
				return err
			}
			cancelTimer := time.NewTimer(10 * time.Second)
			defer cancelTimer.Stop()
			for {
				select {
				case <-result:
					return nil
				case _, ok := <-acp.updates:
					if !ok {
						return errors.New("ACP process closed")
					}
				case <-cancelTimer.C:
					return errors.New("ACP prompt did not stop after cancellation")
				}
			}
		case update, ok := <-acp.updates:
			if !ok {
				return errors.New("ACP process closed")
			}
			var updateErr error
			cumulative, updateErr = handleACPUpdate(conn, writeMu, helixSessionID, acpSessionID, requestID, messageID, cumulative, update)
			if updateErr != nil {
				return updateErr
			}
		}
	}
}

func handleACPUpdate(conn *websocket.Conn, writeMu *sync.Mutex, helixSessionID, acpSessionID, requestID, messageID, cumulative string, update rpcMessage) (string, error) {
	if update.Method != "session/update" {
		return cumulative, nil
	}
	var params struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Content       struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(update.Params, &params) != nil || params.Update.SessionUpdate != "agent_message_chunk" || params.Update.Content.Type != "text" {
		return cumulative, nil
	}
	cumulative += params.Update.Content.Text
	err := sendEvent(conn, writeMu, helixSessionID, "message_added", map[string]any{
		"acp_thread_id": acpSessionID, "message_id": messageID, "request_id": requestID,
		"role": "assistant", "content": cumulative,
	})
	return cumulative, err
}

func syncURL(apiURL, sessionID string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/external-agents/sync"
	u.RawQuery = url.Values{"session_id": []string{sessionID}}.Encode()
	return u.String(), nil
}

func main() {
	ctx := context.Background()
	sessionID := os.Getenv("HELIX_SESSION_ID")
	token := os.Getenv("ZED_HELIX_TOKEN")
	apiURL := os.Getenv("HELIX_API_URL")
	if sessionID == "" || token == "" || apiURL == "" {
		log.Fatal("HELIX_SESSION_ID, ZED_HELIX_TOKEN, and HELIX_API_URL are required")
	}
	userToken := os.Getenv("USER_API_TOKEN")
	if userToken == "" {
		log.Fatal("USER_API_TOKEN is required")
	}
	mcpServers, err := loadGooseConfig(ctx, apiURL, sessionID, userToken)
	if err != nil {
		log.Fatal(err)
	}
	cwd := "/home/retro/work"
	if primary := os.Getenv("HELIX_PRIMARY_REPO_NAME"); primary != "" {
		cwd = filepath.Join(cwd, primary)
	}
	acp, err := startACP(ctx, getenv("HELIX_ACP_COMMAND", "goose acp"))
	if err != nil {
		log.Fatal(err)
	}
	acpSessionID, err := prepareACP(ctx, acp, cwd, filepath.Join("/home/retro/work", ".helix-acp-session.json"), mcpServers)
	if err != nil {
		log.Fatal(err)
	}
	wsURL, err := syncURL(apiURL, sessionID)
	if err != nil {
		log.Fatal(err)
	}
	dialer := websocket.Dialer{}
	if strings.EqualFold(os.Getenv("ZED_HELIX_SKIP_TLS_VERIFY"), "true") {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	commands := make(chan command, 8)
	go func() {
		for {
			var cmd command
			if conn.ReadJSON(&cmd) != nil {
				close(commands)
				return
			}
			commands <- cmd
		}
	}()
	var writeMu sync.Mutex
	if err := sendEvent(conn, &writeMu, sessionID, "agent_ready", map[string]any{"agent_name": "goose", "acp_thread_id": acpSessionID}); err != nil {
		log.Fatal(err)
	}
	for cmd := range commands {
		if cmd.Type != "chat_message" {
			continue
		}
		if err := runPrompt(ctx, acp, conn, &writeMu, sessionID, acpSessionID, cmd, commands); err != nil {
			log.Fatal(err)
		}
	}
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
