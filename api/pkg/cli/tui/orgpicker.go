package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// OrgPickerModel lets the user select which organization to work in.
type OrgPickerModel struct {
	api    *APIClient
	orgs   []*types.Organization
	cursor int
	offset int
	loading bool
	err    error
	width  int
	height int
}

type orgsLoadedMsg struct {
	orgs []*types.Organization
}

type orgSelectedMsg struct {
	org *types.Organization
}

func NewOrgPickerModel(api *APIClient) *OrgPickerModel {
	return &OrgPickerModel{
		api:     api,
		loading: true,
	}
}

func (o *OrgPickerModel) Init() tea.Cmd {
	return func() tea.Msg {
		orgs, err := o.api.ListOrganizations(apiCtx())
		if err != nil {
			return errMsg{err}
		}
		return orgsLoadedMsg{orgs: orgs}
	}
}

func (o *OrgPickerModel) SetSize(w, h int) {
	o.width = w
	o.height = h
}

func (o *OrgPickerModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case orgsLoadedMsg:
		o.orgs = msg.orgs
		o.loading = false
		// Auto-select if only one org
		if len(o.orgs) == 1 {
			return func() tea.Msg {
				return orgSelectedMsg{org: o.orgs[0]}
			}
		}
		return nil

	case errMsg:
		o.err = msg.err
		o.loading = false
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if o.cursor < len(o.orgs)-1 {
				o.cursor++
				o.ensureVisible()
			}
		case "k", "up":
			if o.cursor > 0 {
				o.cursor--
				o.ensureVisible()
			}
		case "enter":
			if len(o.orgs) > 0 && o.cursor < len(o.orgs) {
				org := o.orgs[o.cursor]
				return func() tea.Msg {
					return orgSelectedMsg{org: org}
				}
			}
		}
	}
	return nil
}

func (o *OrgPickerModel) ensureVisible() {
	rows := o.height - 5
	if rows < 3 {
		rows = 3
	}
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if o.cursor >= o.offset+rows {
		o.offset = o.cursor - rows + 1
	}
}

func (o *OrgPickerModel) View() string {
	if o.loading {
		return "\n  Loading organizations..."
	}
	if o.err != nil {
		return fmt.Sprintf("\n  %s %v", styleError.Render("Error:"), o.err)
	}
	if len(o.orgs) == 0 {
		return "\n  No organizations found."
	}

	var b strings.Builder

	title := styleHeader.Render("Select an organization")
	b.WriteString("\n  " + title + "\n\n")

	rows := o.height - 5
	if rows < 3 {
		rows = 3
	}
	end := o.offset + rows
	if end > len(o.orgs) {
		end = len(o.orgs)
	}

	for i := o.offset; i < end; i++ {
		org := o.orgs[i]
		name := org.Name
		if name == "" {
			name = org.ID
		}

		line := fmt.Sprintf("  %s", name)

		if i == o.cursor {
			pointer := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
			b.WriteString(pointer)
			b.WriteString(lipgloss.NewStyle().Foreground(colorText).Bold(true).Render(line))
		} else {
			b.WriteString("  ")
			b.WriteString(styleNormal.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}
