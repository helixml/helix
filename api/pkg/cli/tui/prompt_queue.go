package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// QueuedPrompt is a prompt waiting to be sent to the agent.
type QueuedPrompt struct {
	ID        string
	Content   string
	Interrupt bool
	Status    string // "pending", "sending", "sent", "editing"
}

// PromptQueue manages the visible queue of pending prompts above the input bar.
// Prompts can be edited while queued (which pauses sending).
type PromptQueue struct {
	mu       sync.Mutex
	prompts  []*QueuedPrompt
	editing  int  // index of prompt being edited, -1 if none
	cursor   int  // selected prompt index for navigation
	paused   bool // true when user is editing a queued prompt
	width    int
}

func NewPromptQueue() *PromptQueue {
	return &PromptQueue{
		editing: -1,
		cursor:  -1,
	}
}

func (pq *PromptQueue) SetWidth(w int) {
	pq.width = w
}

// Add queues a new prompt. Interrupts are inserted before non-interrupts.
func (pq *PromptQueue) Add(id, content string, interrupt bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	p := &QueuedPrompt{
		ID:        id,
		Content:   content,
		Interrupt: interrupt,
		Status:    "pending",
	}

	if interrupt {
		// Insert before the first non-interrupt prompt
		insertAt := 0
		for i, existing := range pq.prompts {
			if !existing.Interrupt {
				insertAt = i
				break
			}
			insertAt = i + 1
		}
		pq.prompts = append(pq.prompts[:insertAt], append([]*QueuedPrompt{p}, pq.prompts[insertAt:]...)...)
	} else {
		pq.prompts = append(pq.prompts, p)
	}
}

// ToggleInterrupt toggles the interrupt flag on the selected prompt
// and re-sorts (interrupts above non-interrupts).
func (pq *PromptQueue) ToggleInterrupt(idx int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if idx < 0 || idx >= len(pq.prompts) {
		return
	}

	p := pq.prompts[idx]
	p.Interrupt = !p.Interrupt

	// Remove and re-insert in correct position
	pq.prompts = append(pq.prompts[:idx], pq.prompts[idx+1:]...)

	if p.Interrupt {
		// Insert before first non-interrupt
		insertAt := 0
		for i, existing := range pq.prompts {
			if !existing.Interrupt {
				insertAt = i
				break
			}
			insertAt = i + 1
		}
		pq.prompts = append(pq.prompts[:insertAt], append([]*QueuedPrompt{p}, pq.prompts[insertAt:]...)...)
	} else {
		pq.prompts = append(pq.prompts, p)
	}
}

// Remove removes a prompt by index.
func (pq *PromptQueue) Remove(idx int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if idx >= 0 && idx < len(pq.prompts) {
		pq.prompts = append(pq.prompts[:idx], pq.prompts[idx+1:]...)
		if pq.editing == idx {
			pq.editing = -1
			pq.paused = false
		}
		if pq.cursor >= len(pq.prompts) {
			pq.cursor = len(pq.prompts) - 1
		}
	}
}

// StartEdit begins editing a queued prompt (pauses sending).
func (pq *PromptQueue) StartEdit(idx int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if idx >= 0 && idx < len(pq.prompts) {
		pq.editing = idx
		pq.paused = true
		pq.prompts[idx].Status = "editing"
	}
}

// FinishEdit finishes editing (resumes sending).
func (pq *PromptQueue) FinishEdit(idx int, newContent string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if idx >= 0 && idx < len(pq.prompts) {
		pq.prompts[idx].Content = newContent
		pq.prompts[idx].Status = "pending"
		pq.editing = -1
		pq.paused = false
	}
}

// CancelEdit cancels editing without changing content.
func (pq *PromptQueue) CancelEdit() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if pq.editing >= 0 && pq.editing < len(pq.prompts) {
		pq.prompts[pq.editing].Status = "pending"
	}
	pq.editing = -1
	pq.paused = false
}

// MarkSent marks a prompt as sent and removes it from the queue.
func (pq *PromptQueue) MarkSent(id string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, p := range pq.prompts {
		if p.ID == id {
			pq.prompts = append(pq.prompts[:i], pq.prompts[i+1:]...)
			return
		}
	}
}

// IsPaused returns true when editing is in progress (don't send).
func (pq *PromptQueue) IsPaused() bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.paused
}

// IsEmpty returns true if there are no queued prompts.
func (pq *PromptQueue) IsEmpty() bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.prompts) == 0
}

// Count returns the number of queued prompts.
func (pq *PromptQueue) Count() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.prompts)
}

// NextPending returns the next prompt to send, or nil if paused or empty.
func (pq *PromptQueue) NextPending() *QueuedPrompt {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if pq.paused {
		return nil
	}
	for _, p := range pq.prompts {
		if p.Status == "pending" {
			return p
		}
	}
	return nil
}

// View renders the prompt queue above the input bar.
// Returns empty string if no prompts are queued.
func (pq *PromptQueue) View() string {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.prompts) == 0 {
		return ""
	}

	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Foreground(colorWarning)
	label := fmt.Sprintf("Queued prompts (%d)", len(pq.prompts))
	if pq.paused {
		label += " — paused (editing)"
	}
	b.WriteString("  " + headerStyle.Render(label) + "\n")

	for i, p := range pq.prompts {
		line := pq.renderPrompt(i, p)
		b.WriteString(line + "\n")
	}

	return b.String()
}

func (pq *PromptQueue) renderPrompt(idx int, p *QueuedPrompt) string {
	width := pq.width - 8
	if width < 20 {
		width = 60
	}

	// Status icon
	var icon string
	switch p.Status {
	case "pending":
		icon = styleDim.Render("○")
	case "sending":
		icon = lipgloss.NewStyle().Foreground(colorPrimary).Render("◉")
	case "editing":
		icon = lipgloss.NewStyle().Foreground(colorWarning).Render("✎")
	default:
		icon = styleDim.Render("·")
	}

	// Interrupt badge
	badge := ""
	if p.Interrupt {
		badge = lipgloss.NewStyle().Foreground(colorWarning).Render(" [interrupt]")
	}

	content := truncate(p.Content, width)

	// Highlight if selected
	selected := idx == pq.cursor
	if selected {
		pointer := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
		return fmt.Sprintf("  %s%s %s%s", pointer, icon, content, badge)
	}

	return fmt.Sprintf("    %s %s%s", icon, content, badge)
}

// ViewHeight returns the number of lines the queue renders.
func (pq *PromptQueue) ViewHeight() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.prompts) == 0 {
		return 0
	}
	return len(pq.prompts) + 1 // header + prompts
}
