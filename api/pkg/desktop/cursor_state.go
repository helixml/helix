// Package desktop provides shared cursor state for screenshot compositing.
package desktop

import (
	"sync"
)

// CursorState holds the current cursor position and shape.
// This is updated by the VideoStreamer/cursor listener and read by screenshot functions.
type CursorState struct {
	mu         sync.RWMutex
	x, y       int32  // Cursor position
	cursorName string // CSS cursor name (e.g., "default", "pointer", "text")
}

// globalCursorState is a singleton for sharing cursor state across components.
// This allows the VideoStreamer to update cursor state and screenshot functions to read it.
var globalCursorState = &CursorState{cursorName: "default"}

// GetGlobalCursorState returns the shared cursor state.
func GetGlobalCursorState() *CursorState {
	return globalCursorState
}

// Update updates the cursor state.
func (c *CursorState) Update(x, y int32, cursorName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.x = x
	c.y = y
	if cursorName != "" {
		c.cursorName = cursorName
	}
}

// UpdatePosition updates just the cursor position.
func (c *CursorState) UpdatePosition(x, y int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.x = x
	c.y = y
}

// UpdateShape updates just the cursor shape.
func (c *CursorState) UpdateShape(cursorName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cursorName != "" {
		c.cursorName = cursorName
	}
}

// Get returns the current cursor state.
func (c *CursorState) Get() (x, y int32, cursorName string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.x, c.y, c.cursorName
}
