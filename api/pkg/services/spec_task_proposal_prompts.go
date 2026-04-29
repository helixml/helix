package services

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
)

// ProposalDecisionPromptData is the template input for delivering a user's
// decision on an agent proposal back into the agent's session as a user-turn
// message. Mirrors the shape of ApprovalPromptData / RevisionPromptData, etc.
type ProposalDecisionPromptData struct {
	DecidedByEmail string // user identity who approved/rejected; falls back to user ID
	UserComment    string // free-form comment from the decision UI (Reject reason, Send Back feedback, etc.)
	UserEdits      string // human-readable summary of any payload edits the user made before approving

	// PR-proposal-specific
	HeadBranch string
	BaseBranch string
	PRURL      string
	PRNumber   int

	// Spec-task-proposal-specific
	ResultTaskID string
	TaskName     string
}

var prProposalApprovedPromptTemplate = template.Must(template.New("prProposalApproved").Parse(
	`# PR Proposal Approved

Speak English.

Your proposal to open a pull request was approved by {{.DecidedByEmail}}.

**Branch:** {{.HeadBranch}} → {{.BaseBranch}}
{{if .PRURL}}**PR:** {{.PRURL}}{{if .PRNumber}} (#{{.PRNumber}}){{end}}
{{end}}{{if .UserComment}}**Reviewer note:** {{.UserComment}}
{{end}}{{if .UserEdits}}The reviewer adjusted your proposal before approving:
{{.UserEdits}}
{{end}}
You may continue working. If you want to open more PRs for this task, use ` + "`propose_pull_request`" + ` again. When the work is finished, call ` + "`mark_task_complete`" + `.
`))

var prProposalRejectedPromptTemplate = template.Must(template.New("prProposalRejected").Parse(
	`# PR Proposal Rejected

Speak English.

Your proposal to open a pull request was rejected by {{.DecidedByEmail}}.

**Branch you proposed:** {{.HeadBranch}} → {{.BaseBranch}}
{{if .UserComment}}**Reason:** {{.UserComment}}
{{end}}
Do not retry the same proposal. Address the feedback (in the design docs and/or your code), then either propose a corrected PR or continue with the existing approach.
`))

var specTaskProposalApprovedPromptTemplate = template.Must(template.New("specTaskProposalApproved").Parse(
	`# Spec Task Proposal Approved

Speak English.

Your proposal to create a follow-up task was approved by {{.DecidedByEmail}}.

**New task:** {{.ResultTaskID}}{{if .TaskName}} — {{.TaskName}}{{end}}
{{if .UserEdits}}The reviewer adjusted your proposal before approving:
{{.UserEdits}}
{{end}}{{if .UserComment}}**Reviewer note:** {{.UserComment}}
{{end}}
The task is now in the project backlog. Continue with your current task.
`))

var specTaskProposalRejectedPromptTemplate = template.Must(template.New("specTaskProposalRejected").Parse(
	`# Spec Task Proposal Rejected

Speak English.

Your proposal to create a new task{{if .TaskName}} ({{.TaskName}}){{end}} was rejected by {{.DecidedByEmail}}.

{{if .UserComment}}**Reason:** {{.UserComment}}
{{end}}
Continue with your current task. Do not re-propose the same follow-up.
`))

var markCompleteConfirmedPromptTemplate = template.Must(template.New("markCompleteConfirmed").Parse(
	`# Task Marked Done

Speak English.

{{.DecidedByEmail}} confirmed your mark-complete proposal. The task has been moved to **done**.

No further action required. Do not push more changes.
`))

var markCompleteSentBackPromptTemplate = template.Must(template.New("markCompleteSentBack").Parse(
	`# Mark-Complete Sent Back

Speak English.

{{.DecidedByEmail}} reviewed your mark-complete proposal and sent it back with feedback:

{{if .UserComment}}{{.UserComment}}{{else}}(no comment provided){{end}}

Address the feedback. When you are ready, call ` + "`mark_task_complete`" + ` again.
`))

// BuildProposalDecisionPrompt renders the appropriate prompt template for a
// user's decision on an agent proposal, selecting by Kind + Status. Returns
// an error if the (kind, status) pair is not a valid decision combination.
func BuildProposalDecisionPrompt(proposal *types.SpecTaskProposal, decidedByEmail string) (string, error) {
	if proposal == nil {
		return "", fmt.Errorf("proposal is nil")
	}

	data := ProposalDecisionPromptData{
		DecidedByEmail: decidedByEmail,
		UserComment:    proposal.DecisionComment,
		UserEdits:      summarisePayloadEdits(proposal),

		HeadBranch:   proposal.PRHeadBranch,
		BaseBranch:   proposal.PRBaseBranch,
		PRURL:        proposal.ResultPRURL,
		ResultTaskID: proposal.ResultTaskID,
		TaskName:     proposal.TaskName,
	}
	if data.DecidedByEmail == "" {
		data.DecidedByEmail = "the reviewer"
	}

	var tmpl *template.Template
	switch proposal.Kind {
	case types.ProposalKindPullRequest:
		switch proposal.Status {
		case types.ProposalStatusApproved, types.ProposalStatusFailed:
			tmpl = prProposalApprovedPromptTemplate
		case types.ProposalStatusRejected:
			tmpl = prProposalRejectedPromptTemplate
		}
	case types.ProposalKindSpecTask:
		switch proposal.Status {
		case types.ProposalStatusApproved, types.ProposalStatusFailed:
			tmpl = specTaskProposalApprovedPromptTemplate
		case types.ProposalStatusRejected:
			tmpl = specTaskProposalRejectedPromptTemplate
		}
	case types.ProposalKindMarkComplete:
		switch proposal.Status {
		case types.ProposalStatusApproved:
			tmpl = markCompleteConfirmedPromptTemplate
		case types.ProposalStatusRejected:
			tmpl = markCompleteSentBackPromptTemplate
		}
	}
	if tmpl == nil {
		return "", fmt.Errorf("no prompt template for kind=%s status=%s", proposal.Kind, proposal.Status)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render proposal decision prompt: %w", err)
	}
	return buf.String(), nil
}

// summarisePayloadEdits produces a short human-readable summary of the user's
// edits to the proposal payload, for inclusion in the agent-facing message.
// Returns empty string when no edits were made.
func summarisePayloadEdits(p *types.SpecTaskProposal) string {
	if len(p.EditedPayload) == 0 {
		return ""
	}
	// We don't try to be clever — just include the raw edited payload as a
	// fenced JSON block so the agent can see what changed.
	return fmt.Sprintf("```json\n%s\n```", string(p.EditedPayload))
}
