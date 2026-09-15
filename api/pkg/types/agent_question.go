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

type ResolvedQuestion struct {
	PendingQuestion
	Outcome    string            `json:"outcome"`
	Answers    map[string]string `json:"answers,omitempty"`
	ResolvedAt time.Time         `json:"resolved_at"`
}

type QuestionRespondRequest struct {
	Answers map[string]string `json:"answers"`
}

type QuestionActionResponse struct {
	Status string `json:"status"`
}
