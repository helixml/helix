//go:build !cgo

package desktop

import (
	"context"
)

// ClaudeJSONLWatcher stub for non-cgo builds
type ClaudeJSONLWatcher struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type ClaudeInteraction struct {
	UUID       string                 `json:"uuid"`
	ParentUUID string                 `json:"parentUuid,omitempty"`
	Type       string                 `json:"type"`
	SessionID  string                 `json:"sessionId"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type ClaudeMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type ClaudeToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

type ClaudeJSONLWatcherConfig struct {
	WorkDir       string
	ProjectDir    string
	OnInteraction func(interaction *ClaudeInteraction)
	OnError       func(err error)
}

func NewClaudeJSONLWatcher(cfg ClaudeJSONLWatcherConfig) (*ClaudeJSONLWatcher, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &ClaudeJSONLWatcher{ctx: ctx, cancel: cancel}, nil
}

func (w *ClaudeJSONLWatcher) Start() error {
	return nil
}

func (w *ClaudeJSONLWatcher) Stop() error {
	w.cancel()
	return nil
}

func (ci *ClaudeInteraction) GetTextContent() string {
	return ""
}
