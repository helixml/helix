package types

import "time"

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

// QuestionSubmission records an answer or cancel submitted through the API.
// ID is a nonce Helix sends with the forwarded command; only a resolution
// that echoes it back is attributed to By. Content is never compared: in
// Zed's UI a human approving a permission prompt reports the same
// "cancelled" with no answers as an API cancel.
type QuestionSubmission struct {
	ID          string            `json:"id"`
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

// NewResolvedQuestion is the history entry for resolving q, answered by
// answeredBy. The submission is not kept on the entry.
func NewResolvedQuestion(q *PendingQuestion, outcome string, answers map[string]string, answeredBy TurnProvenance) ResolvedQuestion {
	pending := *q
	pending.Submission = nil
	return ResolvedQuestion{
		PendingQuestion: pending,
		Outcome:         outcome,
		Answers:         answers,
		ResolvedAt:      time.Now(),
		AnsweredBy:      answeredBy,
	}
}

// ResolveAnswerer attributes a resolution the agent reported: to the API
// submission only when the agent echoed that submission's nonce, otherwise
// to whoever acted in the agent's own UI.
func (q *PendingQuestion) ResolveAnswerer(submissionID string) TurnProvenance {
	if submissionID != "" && q.Submission != nil && q.Submission.ID == submissionID {
		return q.Submission.By
	}
	return TurnProvenance{Origin: TurnOriginExternalAgent}
}

type QuestionRespondRequest struct {
	Answers map[string]string `json:"answers"`
}

type QuestionActionResponse struct {
	Status string `json:"status"`
}
