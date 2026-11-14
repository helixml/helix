package types

import (
	"maps"
	"time"
)

type UserQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type UserQuestion struct {
	ID                string               `json:"id"`
	Header            string               `json:"header"`
	Question          string               `json:"question"`
	Options           []UserQuestionOption `json:"options"`
	MultiSelect       bool                 `json:"multi_select"`
	AllowCustomAnswer bool                 `json:"allow_custom_answer"`
}

type PendingQuestion struct {
	RequestID     string         `json:"request_id"`
	ThreadID      string         `json:"thread_id"`
	TurnRequestID string         `json:"turn_request_id,omitempty"`
	Source        string         `json:"source"`
	Questions     []UserQuestion `json:"questions"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	AskedAt       time.Time      `json:"asked_at"`
	// Submission is the action Helix's API accepted for this question and
	// forwarded to the agent. Server-written only; cleared on resolution.
	Submission *QuestionSubmission `json:"submission,omitempty"`
}

// QuestionSubmission records an answer or cancel submitted through the API,
// so the resolution the agent reports back can be attributed to its sender.
type QuestionSubmission struct {
	Outcome     string            `json:"outcome"`
	Answers     map[string]string `json:"answers,omitempty"`
	By          TurnProvenance    `json:"by"`
	SubmittedAt time.Time         `json:"submitted_at"`
}

type ResolvedQuestion struct {
	PendingQuestion
	Outcome    string            `json:"outcome"`
	Answers    map[string]string `json:"answers,omitempty"`
	ResolvedAt time.Time         `json:"resolved_at"`
	// AnsweredBy says who resolved the question. Server-set; entries from
	// before it existed read as origin "unknown".
	AnsweredBy TurnProvenance `json:"answered_by"`
}

// NewResolvedQuestion is the history entry for resolving q. The submission
// is folded into AnsweredBy rather than kept on the entry.
func NewResolvedQuestion(q *PendingQuestion, outcome string, answers map[string]string, fallback TurnProvenance) ResolvedQuestion {
	pending := *q
	pending.Submission = nil
	return ResolvedQuestion{
		PendingQuestion: pending,
		Outcome:         outcome,
		Answers:         answers,
		ResolvedAt:      time.Now(),
		AnsweredBy:      q.ResolveAnswerer(outcome, answers, fallback),
	}
}

// ResolveAnswerer attributes a resolution: to the API submission when the
// agent reports exactly the outcome and answers that were submitted, and
// otherwise to fallback (the agent's UI, or Helix settling the question).
func (q *PendingQuestion) ResolveAnswerer(outcome string, answers map[string]string, fallback TurnProvenance) TurnProvenance {
	if q.Submission != nil && q.Submission.Outcome == outcome && maps.Equal(q.Submission.Answers, answers) {
		return q.Submission.By
	}
	return fallback
}

type QuestionRespondRequest struct {
	Answers map[string]string `json:"answers"`
}

type QuestionActionResponse struct {
	Status string `json:"status"`
}
