//go:build !cgo

package desktop

import "fmt"

// ClaudeCodeBridge is a stub for non-CGO builds
type ClaudeCodeBridge struct{}

// ClaudeCodeBridgeConfig is a stub for non-CGO builds
type ClaudeCodeBridgeConfig struct {
	SessionID      string
	TmuxSession    string
	WorkDir        string
	OnAgentReady   func()
	OnMessageAdded func(content string, isComplete bool)
	OnError        func(err error)
}

// NewClaudeCodeBridge returns an error for non-CGO builds
func NewClaudeCodeBridge(cfg ClaudeCodeBridgeConfig) (*ClaudeCodeBridge, error) {
	return nil, fmt.Errorf("ClaudeCodeBridge requires CGO")
}

// Start is a stub
func (b *ClaudeCodeBridge) Start() error {
	return fmt.Errorf("ClaudeCodeBridge requires CGO")
}

// SendMessage is a stub
func (b *ClaudeCodeBridge) SendMessage(message string) error {
	return fmt.Errorf("ClaudeCodeBridge requires CGO")
}

// StopTask is a stub
func (b *ClaudeCodeBridge) StopTask() error {
	return fmt.Errorf("ClaudeCodeBridge requires CGO")
}

// Close is a stub
func (b *ClaudeCodeBridge) Close() error {
	return nil
}
