package agent

type ResponseType string

const (
	ResponseTypeStatus ResponseType = "status"

	ResponseTypeThinkingStart ResponseType = "thinking-start"
	ResponseTypeThinking      ResponseType = "thinking"
	ResponseTypeThinkingEnd   ResponseType = "thinking-end"

	ResponseTypePartialText ResponseType = "partial-text"
	ResponseTypeEnd         ResponseType = "end"

	ResponseTypeInputRequest ResponseType = "input-request"

	ResponseTypeError ResponseType = "error"
)

// Response represents a communication unit from the Agent to the caller/UI.
type Response struct {
	Content string
	Type    ResponseType
}
