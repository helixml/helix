package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PaneState is the serializable state of a single pane or split.
type PaneState struct {
	// Leaf
	TaskID string `json:"task_id,omitempty"`

	// Split
	Dir   string     `json:"dir,omitempty"` // "vertical" or "horizontal"
	Left  *PaneState `json:"left,omitempty"`
	Right *PaneState `json:"right,omitempty"`
}

// TUIState is the full serializable state for detach/reattach.
type TUIState struct {
	ProjectID     string     `json:"project_id"`
	Panes         *PaneState `json:"panes,omitempty"`
	FocusedTaskID string     `json:"focused_task_id,omitempty"`
}

func stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/helix-tui"
	}
	return filepath.Join(home, ".helix", "tui")
}

func statePath() string {
	return filepath.Join(stateDir(), "state.json")
}

// SaveState writes the current TUI state to disk.
func SaveState(state *TUIState) error {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath(), data, 0600)
}

// LoadState reads the saved TUI state from disk. Returns nil if no state exists.
func LoadState() *TUIState {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return nil
	}

	var state TUIState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return &state
}

// ClearState removes the saved state file.
func ClearState() {
	os.Remove(statePath())
}

// SerializePaneTree converts the in-memory pane tree to serializable state.
func SerializePaneTree(node *PaneTree) *PaneState {
	if node == nil {
		return nil
	}

	if node.Chat != nil {
		taskID := ""
		if node.Chat.task != nil {
			taskID = node.Chat.task.ID
		}
		return &PaneState{TaskID: taskID}
	}

	dir := "vertical"
	if node.Dir == SplitHorizontal {
		dir = "horizontal"
	}

	return &PaneState{
		Dir:   dir,
		Left:  SerializePaneTree(node.Left),
		Right: SerializePaneTree(node.Right),
	}
}

// BuildStateFromApp captures the current app state for persistence.
func BuildStateFromApp(a *App) *TUIState {
	state := &TUIState{}

	if a.kanban != nil {
		state.ProjectID = a.kanban.projectID
	}

	if a.panes != nil && a.panes.Root != nil {
		state.Panes = SerializePaneTree(a.panes.Root)
		if chat := a.panes.FocusedChat(); chat != nil && chat.task != nil {
			state.FocusedTaskID = chat.task.ID
		}
	}

	return state
}
