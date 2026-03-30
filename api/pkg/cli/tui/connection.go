package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ConnState represents the connection state to the Helix API.
type ConnState int

const (
	ConnConnected    ConnState = iota
	ConnDisconnected
)

// ConnectionManager tracks API connection health.
type ConnectionManager struct {
	mu            sync.Mutex
	state         ConnState
	lastContact   time.Time
	failCount     int
	lastError     error
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		state:       ConnConnected,
		lastContact: time.Now(),
	}
}

// RecordSuccess marks a successful API call.
func (cm *ConnectionManager) RecordSuccess() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.state = ConnConnected
	cm.lastContact = time.Now()
	cm.failCount = 0
	cm.lastError = nil
}

// RecordFailure marks a failed API call.
func (cm *ConnectionManager) RecordFailure(err error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.failCount++
	cm.lastError = err
	if cm.failCount >= 2 {
		cm.state = ConnDisconnected
	}
}

// IsConnected returns true if we think the API is reachable.
func (cm *ConnectionManager) IsConnected() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.state == ConnConnected
}

// DisconnectedDuration returns how long we've been disconnected.
func (cm *ConnectionManager) DisconnectedDuration() time.Duration {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.state == ConnConnected {
		return 0
	}
	return time.Since(cm.lastContact)
}

// RenderBar renders the mosh-style disconnection bar.
// Returns empty string if connected.
func (cm *ConnectionManager) RenderBar(width int) string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state == ConnConnected {
		return ""
	}

	elapsed := time.Since(cm.lastContact)
	var timeStr string
	if elapsed < time.Minute {
		timeStr = fmt.Sprintf("%d seconds", int(elapsed.Seconds()))
	} else if elapsed < time.Hour {
		timeStr = fmt.Sprintf("%d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	} else {
		timeStr = fmt.Sprintf("%d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
	}

	msg := fmt.Sprintf("helix: Last contact %s ago. [To quit: Ctrl-^ .]", timeStr)

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("130")).
		Bold(true).
		Width(width).
		Align(lipgloss.Center)

	top := lipgloss.NewStyle().
		Foreground(lipgloss.Color("130")).
		Render(strings.Repeat("─", width))
	bottom := top

	return top + "\n" + barStyle.Render(msg) + "\n" + bottom
}
