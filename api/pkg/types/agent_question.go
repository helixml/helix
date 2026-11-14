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
}

// QuestionSubmission records an answer or cancel submitted through the API
// for the pending question RequestID. It lives in its own server-only column
// (Interaction.QuestionSubmission), never in API responses or frontend
// publishes, and the agent has no way to write it. ID is a nonce Helix sends
// with the forwarded command; see Answerer.
type QuestionSubmission struct {
	ID          string            `json:"id"`
	RequestID   string            `json:"request_id"`
	Outcome     string            `json:"outcome"`
	Answers     map[string]string `json:"answers,omitempty"`
	By          TurnProvenance    `json:"by"`
	SubmittedAt time.Time         `json:"submitted_at"`
}

// Answerer attributes a resolution the agent reported for requestID. It is
// credited to the API sender only when the agent echoed this submission's
// nonce and reported the outcome that was submitted; content is never
// compared, because in Zed's UI a human approving a permission prompt reports
// the same "cancelled" with no answers as an API cancel. Anything else was
// done in the agent's own UI. Safe on a nil submission.
func (s *QuestionSubmission) Answerer(requestID, submissionID, outcome string) TurnProvenance {
	if s != nil && submissionID != "" && s.ID == submissionID && s.RequestID == requestID && s.Outcome == outcome {
		return s.By
	}
	return TurnProvenance{Origin: TurnOriginExternalAgent}
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
// answeredBy.
func NewResolvedQuestion(q *PendingQuestion, outcome string, answers map[string]string, answeredBy TurnProvenance) ResolvedQuestion {
	return ResolvedQuestion{
		PendingQuestion: *q,
		Outcome:         outcome,
		Answers:         answers,
		ResolvedAt:      time.Now(),
		AnsweredBy:      answeredBy,
	}
}

type QuestionRespondRequest struct {
	Answers map[string]string `json:"answers"`
}

type QuestionActionResponse struct {
	Status string `json:"status"`
}

// SubmissionFor is the API submission in flight for the pending question
// requestID, or nil.
func (i *Interaction) SubmissionFor(requestID string) *QuestionSubmission {
	if i.QuestionSubmission == nil || i.QuestionSubmission.RequestID != requestID {
		return nil
	}
	return i.QuestionSubmission
}
