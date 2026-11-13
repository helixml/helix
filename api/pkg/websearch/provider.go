package websearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination provider_mocks.go -package $GOPACKAGE

// Provider serves the Exa-compatible API. Handlers, agent skills and the CLI
// depend only on this; a deployment picks the implementation (Helix's own
// stack, or Exa) in configuration.
//
// Each method returns *Error for failures the caller should see with a
// specific status and tag; any other error is reported as a 500.
type Provider interface {
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
	// StreamSearch serves /search with stream=true and an outputSchema. emit
	// is called for every event, ending with a SearchEventDone event.
	StreamSearch(ctx context.Context, req *SearchRequest, emit func(SearchEvent) error) error
	Contents(ctx context.Context, req *ContentsRequest) (*ContentsResponse, error)
	FindSimilar(ctx context.Context, req *FindSimilarRequest) (*FindSimilarResponse, error)
	Answer(ctx context.Context, req *AnswerRequest) (*AnswerResponse, error)
	// StreamAnswer serves /answer with stream=true.
	StreamAnswer(ctx context.Context, req *AnswerRequest, emit func(AnswerEvent) error) error
}

// Error tags for the {requestId, error, tag} envelope.
const (
	TagInvalidRequest  = "INVALID_REQUEST"
	TagFeatureDisabled = "FEATURE_DISABLED"
	TagNotFound        = "NOT_FOUND"
	TagUpstreamError   = "UPSTREAM_ERROR"
	TagInternalError   = "INTERNAL_ERROR"
	TagNoMoreCredits   = "NO_MORE_CREDITS"
)

// Error is a failure with an Exa-style status code and machine-readable tag.
type Error struct {
	Status  int
	Tag     string
	Message string
}

func (e *Error) Error() string { return e.Message }

// ErrorResponse is Exa's error body: {requestId, error, tag}.
type ErrorResponse struct {
	RequestID string `json:"requestId"`
	Error     string `json:"error"`
	Tag       string `json:"tag"`
}

// InvalidRequest is a 400 for a request that breaks the API contract.
func InvalidRequest(format string, args ...any) *Error {
	return &Error{Status: http.StatusBadRequest, Tag: TagInvalidRequest, Message: fmt.Sprintf(format, args...)}
}

// FeatureDisabled is a 400 for a recognised field this deployment cannot
// honour. Fields that change what results mean are never silently ignored.
func FeatureDisabled(path, reason string) *Error {
	return &Error{Status: http.StatusBadRequest, Tag: TagFeatureDisabled, Message: fmt.Sprintf("%s is not supported: %s", path, reason)}
}

// AsError returns err as *Error, wrapping unknown errors as a 500.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Status: http.StatusInternalServerError, Tag: TagInternalError, Message: err.Error()}
}

// Caller identifies a request and who it is made for, so it is billed to
// them, recorded in their usage log, and the LLM and embedding calls a
// provider makes are attributed to them. RequestID is the id the client
// sees and the credit transaction and usage log record.
type Caller struct {
	UserID    string
	OrgID     string
	RequestID string
	Source    types.WebSearchSource
	// APIKeyName names the API key an API call authenticated with, if any.
	APIKeyName string
	// System marks Helix's own runner token, which is never billed, as with
	// inference.
	System bool
}

type callerKey struct{}

func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

func CallerFrom(ctx context.Context) Caller {
	c, _ := ctx.Value(callerKey{}).(Caller)
	return c
}
