package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// ReviewMode tracks the current review interaction mode.
type ReviewMode int

const (
	ReviewViewing   ReviewMode = iota // Scrolling through spec
	ReviewSelecting                   // Visual line selection (shift+V)
	ReviewCommenting                  // Typing a comment on selection
)

// ReviewModel is the spec review UI for tasks in spec_review status.
type ReviewModel struct {
	api  *APIClient
	task *types.SpecTask

	// Content
	lines      []string // spec content split into lines
	sections   []string // section headers for navigation
	scrollTop  int      // first visible line index
	cursorLine int      // current cursor line

	// Selection (visual mode)
	mode       ReviewMode
	selectFrom int // start of selection
	selectTo   int // end of selection

	// Comment input
	commentInput *InputModel

	// Layout
	width  int
	height int
	err    error
}

type specApprovedMsg struct {
	taskID string
}

type specRevisionRequestedMsg struct {
	taskID  string
	comment string
}

func NewReviewModel(api *APIClient, task *types.SpecTask) *ReviewModel {
	r := &ReviewModel{
		api:          api,
		task:         task,
		commentInput: NewInputModel(),
	}

	// Parse spec content into lines
	r.parseSpec()
	return r
}

func (r *ReviewModel) parseSpec() {
	var content strings.Builder

	if r.task.RequirementsSpec != "" {
		content.WriteString("## Requirements Specification\n\n")
		content.WriteString(r.task.RequirementsSpec)
		content.WriteString("\n\n")
	}

	if r.task.TechnicalDesign != "" {
		content.WriteString("## Technical Design\n\n")
		content.WriteString(r.task.TechnicalDesign)
		content.WriteString("\n\n")
	}

	if r.task.ImplementationPlan != "" {
		content.WriteString("## Implementation Plan\n\n")
		content.WriteString(r.task.ImplementationPlan)
	}

	r.lines = strings.Split(content.String(), "\n")

	// Extract section headers
	for _, line := range r.lines {
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			r.sections = append(r.sections, line)
		}
	}
}

func (r *ReviewModel) Init() tea.Cmd {
	return nil
}

func (r *ReviewModel) SetSize(w, h int) {
	r.width = w
	r.height = h
	r.commentInput.SetWidth(w)
}

func (r *ReviewModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case errMsg:
		r.err = msg.err
		return nil

	case tea.KeyMsg:
		switch r.mode {
		case ReviewViewing:
			return r.updateViewing(msg)
		case ReviewSelecting:
			return r.updateSelecting(msg)
		case ReviewCommenting:
			return r.updateCommenting(msg)
		}
	}
	return nil
}

func (r *ReviewModel) updateViewing(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if r.cursorLine < len(r.lines)-1 {
			r.cursorLine++
			r.ensureVisible()
		}
	case "k", "up":
		if r.cursorLine > 0 {
			r.cursorLine--
			r.ensureVisible()
		}
	case "G":
		r.cursorLine = len(r.lines) - 1
		r.ensureVisible()
	case "g":
		r.cursorLine = 0
		r.scrollTop = 0

	case "V": // shift+V: start visual line selection
		r.mode = ReviewSelecting
		r.selectFrom = r.cursorLine
		r.selectTo = r.cursorLine

	case "a": // approve specs
		return r.approveSpecs()

	case "r": // request changes
		r.mode = ReviewCommenting
		r.selectFrom = 0
		r.selectTo = len(r.lines) - 1
		r.commentInput.SetFocused(true)

	case "c": // comment (starts selection first)
		r.mode = ReviewSelecting
		r.selectFrom = r.cursorLine
		r.selectTo = r.cursorLine
	}
	return nil
}

func (r *ReviewModel) updateSelecting(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if r.selectTo < len(r.lines)-1 {
			r.selectTo++
			r.cursorLine = r.selectTo
			r.ensureVisible()
		}
	case "k", "up":
		if r.selectTo > 0 {
			r.selectTo--
			r.cursorLine = r.selectTo
			r.ensureVisible()
		}

	case "c", "enter": // start commenting on selection
		r.mode = ReviewCommenting
		r.commentInput.SetFocused(true)

	case "esc": // cancel selection
		r.mode = ReviewViewing
	}
	return nil
}

func (r *ReviewModel) updateCommenting(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		if !r.commentInput.IsEmpty() {
			r.commentInput.Clear()
			return nil
		}
		r.mode = ReviewViewing
		r.commentInput.SetFocused(false)
		return nil

	case "enter":
		if !r.commentInput.IsEmpty() {
			comment := r.commentInput.Value()
			r.commentInput.Clear()
			r.commentInput.SetFocused(false)
			r.mode = ReviewViewing
			return r.submitComment(comment)
		}

	case "backspace":
		r.commentInput.Backspace()

	default:
		if msg.Type == tea.KeyRunes {
			r.commentInput.InsertRunes([]rune(msg.String()))
		} else if msg.String() == " " {
			r.commentInput.InsertRunes([]rune{' '})
		}
	}
	return nil
}

func (r *ReviewModel) ensureVisible() {
	viewHeight := r.height - 5 // header + status
	if viewHeight < 1 {
		viewHeight = 1
	}
	if r.cursorLine < r.scrollTop {
		r.scrollTop = r.cursorLine
	}
	if r.cursorLine >= r.scrollTop+viewHeight {
		r.scrollTop = r.cursorLine - viewHeight + 1
	}
}

func (r *ReviewModel) approveSpecs() tea.Cmd {
	taskID := r.task.ID
	return func() tea.Msg {
		err := r.api.client.MakeRequest(apiCtx(), "POST",
			"/spec-tasks/"+taskID+"/approve-specs", nil, nil)
		if err != nil {
			return errMsg{err}
		}
		return specApprovedMsg{taskID: taskID}
	}
}

func (r *ReviewModel) submitComment(comment string) tea.Cmd {
	// Extract selected text for context
	from := r.selectFrom
	to := r.selectTo
	if from > to {
		from, to = to, from
	}
	selectedText := strings.Join(r.lines[from:to+1], "\n")

	taskID := r.task.ID
	return func() tea.Msg {
		_ = selectedText // TODO: include in API call
		return specRevisionRequestedMsg{taskID: taskID, comment: comment}
	}
}

func (r *ReviewModel) View() string {
	if r.width < 20 {
		return "Too narrow"
	}

	// Header
	header := r.renderHeader()

	// Content area
	viewHeight := r.height - 5
	if viewHeight < 1 {
		viewHeight = 1
	}
	content := r.renderContent(viewHeight)

	// Status / input area
	var bottom string
	switch r.mode {
	case ReviewViewing:
		bottom = r.renderViewingStatus()
	case ReviewSelecting:
		bottom = r.renderSelectingStatus()
	case ReviewCommenting:
		bottom = r.renderCommentInput()
	}

	return header + "\n" + content + "\n" + bottom
}

func (r *ReviewModel) renderHeader() string {
	name := taskDisplayName(r.task)
	status := string(r.task.Status)

	title := styleHeader.Render("Review: " + name)
	meta := styleDim.Render(status)

	return title + " " + meta
}

func (r *ReviewModel) renderContent(height int) string {
	var b strings.Builder

	from, to := r.selectFrom, r.selectTo
	if from > to {
		from, to = to, from
	}

	end := r.scrollTop + height
	if end > len(r.lines) {
		end = len(r.lines)
	}

	lineNumWidth := len(fmt.Sprintf("%d", len(r.lines)))

	for i := r.scrollTop; i < end; i++ {
		line := r.lines[i]
		if len(line) > r.width-lineNumWidth-4 {
			line = line[:r.width-lineNumWidth-7] + "..."
		}

		// Line number
		lineNum := styleDim.Render(fmt.Sprintf("%*d ", lineNumWidth, i+1))

		// Determine line style
		isSelected := r.mode == ReviewSelecting && i >= from && i <= to
		isCursor := i == r.cursorLine

		var styledLine string
		if isSelected {
			styledLine = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(colorText).
				Width(r.width - lineNumWidth - 3).
				Render(line)
		} else if isCursor && r.mode == ReviewViewing {
			styledLine = lipgloss.NewStyle().
				Background(colorSelected).
				Foreground(colorText).
				Width(r.width - lineNumWidth - 3).
				Render(line)
		} else if strings.HasPrefix(line, "## ") {
			styledLine = styleHeader.Render(line)
		} else if strings.HasPrefix(line, "### ") {
			styledLine = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(line)
		} else if strings.HasPrefix(line, "- [ ] ") {
			styledLine = styleDim.Render("☐ ") + line[6:]
		} else if strings.HasPrefix(line, "- [x] ") {
			styledLine = lipgloss.NewStyle().Foreground(colorSuccess).Render("☑ ") + line[6:]
		} else {
			styledLine = line
		}

		b.WriteString(lineNum + styledLine + "\n")
	}

	// Pad remaining
	for i := end - r.scrollTop; i < height; i++ {
		b.WriteString(styleDim.Render("~") + "\n")
	}

	return b.String()
}

func (r *ReviewModel) renderViewingStatus() string {
	pos := fmt.Sprintf("Line %d/%d", r.cursorLine+1, len(r.lines))
	help := "shift+V: select  c: comment  a: approve  r: request changes  esc: back"
	return styleDim.Render(pos+"  "+help)
}

func (r *ReviewModel) renderSelectingStatus() string {
	from, to := r.selectFrom, r.selectTo
	if from > to {
		from, to = to, from
	}
	sel := fmt.Sprintf("Selected lines %d-%d", from+1, to+1)
	help := "j/k: extend selection  c/enter: comment  esc: cancel"
	return lipgloss.NewStyle().Foreground(colorPrimary).Render(sel) + "  " + styleDim.Render(help)
}

func (r *ReviewModel) renderCommentInput() string {
	prompt := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("Comment❯ ")
	text := r.commentInput.Value() + "█"
	return prompt + text + "\n" + styleDim.Render("  enter: submit  esc: cancel")
}
