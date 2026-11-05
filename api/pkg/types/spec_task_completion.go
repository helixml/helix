package types

// CompletionDecisionRequest is the user's answer to an agent's request to mark
// its task done: "approve" moves it to done, "reject" sends it back with the
// comment as feedback for the agent.
type CompletionDecisionRequest struct {
	Decision string `json:"decision"` // "approve" or "reject"
	Comment  string `json:"comment,omitempty"`
}

const (
	CompletionDecisionApprove = "approve"
	CompletionDecisionReject  = "reject"
)
