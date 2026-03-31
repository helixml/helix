package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/helixml/helix/api/pkg/types"
)

func TestStatusToColumn(t *testing.T) {
	tests := []struct {
		status   types.SpecTaskStatus
		expected KanbanColumn
	}{
		{types.TaskStatusBacklog, ColBacklog},
		{types.TaskStatusQueuedSpecGeneration, ColPlanning},
		{types.TaskStatusSpecGeneration, ColPlanning},
		{types.TaskStatusSpecReview, ColPlanning},
		{types.TaskStatusSpecRevision, ColPlanning},
		{types.TaskStatusSpecApproved, ColPlanning},
		{types.TaskStatusSpecFailed, ColPlanning},
		{types.TaskStatusQueuedImplementation, ColInProgress},
		{types.TaskStatusImplementationQueued, ColInProgress},
		{types.TaskStatusImplementation, ColInProgress},
		{types.TaskStatusImplementationFailed, ColInProgress},
		{types.TaskStatusImplementationReview, ColReview},
		{types.TaskStatusPullRequest, ColReview},
		{types.TaskStatusDone, ColDone},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := statusToColumn(tt.status)
			if got != tt.expected {
				t.Errorf("statusToColumn(%q) = %d (%s), want %d (%s)",
					tt.status, got, got.Title(), tt.expected, tt.expected.Title())
			}
		})
	}
}

func TestKanbanColumnScrolling(t *testing.T) {
	k := &KanbanModel{height: 20}

	// Add 30 tasks to backlog
	for i := 0; i < 30; i++ {
		k.columns[ColBacklog] = append(k.columns[ColBacklog], &types.SpecTask{
			ID:   "spt_" + string(rune('a'+i%26)),
			Name: "Task",
		})
	}

	// Initial state
	if k.rowIdx[ColBacklog] != 0 {
		t.Error("cursor should start at 0")
	}
	if k.scrollOff[ColBacklog] != 0 {
		t.Error("scroll should start at 0")
	}

	// Move down past visible area
	ch := k.cardHeight()
	for i := 0; i < ch+5; i++ {
		if k.rowIdx[ColBacklog] < len(k.columns[ColBacklog])-1 {
			k.rowIdx[ColBacklog]++
			k.ensureVisible(ColBacklog)
		}
	}

	if k.scrollOff[ColBacklog] == 0 {
		t.Error("scroll should have advanced")
	}
	if k.rowIdx[ColBacklog] < ch {
		t.Error("cursor should be past initial visible area")
	}

	// Scroll offset should keep cursor visible
	if k.rowIdx[ColBacklog] < k.scrollOff[ColBacklog] ||
		k.rowIdx[ColBacklog] >= k.scrollOff[ColBacklog]+ch {
		t.Error("cursor should be within visible range")
	}
}

func TestKanbanEnterDispatchesByStatus(t *testing.T) {
	k := NewKanbanModel(nil, "proj_1")
	k.columns[ColBacklog] = []*types.SpecTask{{ID: "spt_1", Status: types.TaskStatusBacklog}}
	k.columns[ColPlanning] = []*types.SpecTask{{ID: "spt_2", Status: types.TaskStatusSpecReview}}
	k.columns[ColInProgress] = []*types.SpecTask{{ID: "spt_3", Status: types.TaskStatusImplementation}}

	// Backlog → shows confirmation prompt
	k.colIdx = ColBacklog
	cmd := k.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("backlog enter should not return command (shows prompt first)")
	}
	if k.confirmTask == nil {
		t.Fatal("expected confirmTask to be set")
	}
	// Press 'y' to confirm
	cmd = k.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected command from 'y' confirmation")
	}
	msg := cmd()
	if _, ok := msg.(startPlanningMsg); !ok {
		t.Errorf("confirmed backlog should produce startPlanningMsg, got %T", msg)
	}

	// Planning (spec_review) → openReviewMsg
	k.colIdx = ColPlanning
	cmd = k.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from enter on planning")
	}
	msg = cmd()
	if _, ok := msg.(openReviewMsg); !ok {
		t.Errorf("spec_review enter should produce openReviewMsg, got %T", msg)
	}

	// In Progress → openTaskChatMsg
	k.colIdx = ColInProgress
	cmd = k.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from enter on in progress")
	}
	msg = cmd()
	if _, ok := msg.(openTaskChatMsg); !ok {
		t.Errorf("implementation enter should produce openTaskChatMsg, got %T", msg)
	}
}
