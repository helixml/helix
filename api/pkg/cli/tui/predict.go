package tui

import (
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// PredictiveEcho implements mosh-style predictive local echo for the input area.
// Keystrokes are rendered immediately in a dim color ("predicted"), then
// confirmed when the server acknowledges them. If the prediction was wrong
// (server rejects or modifies), the predicted text is rolled back.
type PredictiveEcho struct {
	mu sync.Mutex

	// confirmed is the text the server has acknowledged
	confirmed string

	// predicted is the locally-echoed text appended after confirmed
	predicted string

	// enabled controls whether predictions are active
	enabled bool
}

func NewPredictiveEcho() *PredictiveEcho {
	return &PredictiveEcho{
		enabled: true,
	}
}

// SetEnabled toggles predictive echo on/off.
func (p *PredictiveEcho) SetEnabled(e bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = e
	if !e {
		p.predicted = ""
	}
}

// AddPredicted appends a character to the predicted buffer.
// Called immediately when the user types.
func (p *PredictiveEcho) AddPredicted(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return
	}
	p.predicted += s
}

// RemovePredicted removes the last character from the predicted buffer.
func (p *PredictiveEcho) RemovePredicted() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.predicted) > 0 {
		p.predicted = p.predicted[:len(p.predicted)-1]
	}
}

// Confirm acknowledges that the server has received all input up to this point.
// The server text becomes the new confirmed state. Predicted text is adjusted:
// - If predicted text is a suffix of server text, it's consumed
// - If server text diverges from what we predicted, predictions are rolled back
func (p *PredictiveEcho) Confirm(serverText string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	oldFull := p.confirmed + p.predicted

	if oldFull == serverText {
		// Perfect prediction — everything confirmed
		p.confirmed = serverText
		p.predicted = ""
		return
	}

	// Check if predicted text extends beyond what server confirmed
	if len(oldFull) > len(serverText) && oldFull[:len(serverText)] == serverText {
		// Server confirmed a prefix, rest is still predicted
		p.confirmed = serverText
		p.predicted = oldFull[len(serverText):]
		return
	}

	// Server diverged — accept server text, clear predictions
	p.confirmed = serverText
	p.predicted = ""
}

// Rollback clears all predicted text (server rejected the input).
func (p *PredictiveEcho) Rollback() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.predicted = ""
}

// FullText returns confirmed + predicted for display.
func (p *PredictiveEcho) FullText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.confirmed + p.predicted
}

// HasPredicted returns true if there's unconfirmed predicted text.
func (p *PredictiveEcho) HasPredicted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.predicted != ""
}

// RenderInput renders the combined input with predictions in dim color.
func (p *PredictiveEcho) RenderInput(confirmedStyle, predictedStyle lipgloss.Style) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := confirmedStyle.Render(p.confirmed)
	if p.predicted != "" {
		result += predictedStyle.Render(p.predicted)
	}
	return result
}

// Reset clears all state.
func (p *PredictiveEcho) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.confirmed = ""
	p.predicted = ""
}
