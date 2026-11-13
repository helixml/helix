package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/auditlog"
	"github.com/helixml/helix/api/pkg/controller"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// chatCompletionErrorBody is OpenAI's error object, so OpenAI SDKs and coding
// agents parse a Helix refusal exactly like one from OpenAI.
type chatCompletionErrorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    any     `json:"code"`
	Param   *string `json:"param"`
}

type chatCompletionErrorResponse struct {
	Error chatCompletionErrorBody `json:"error"`
}

// chatCompletionFailure is the answer to one failed chat completion.
type chatCompletionFailure struct {
	status int
	body   chatCompletionErrorBody
	// header carries the provider's back-off headers on a 429.
	header http.Header
}

const (
	errorTypeInvalidRequest = "invalid_request_error"
	errorTypeRateLimit      = "rate_limit_error"
	errorTypeInsufficient   = "insufficient_quota"
	errorTypeUpstream       = "upstream_error"
	errorTypeServer         = "server_error"
)

// chatCompletionErrorScope is what the handler knows about the failed request:
// what the caller asked for (to word model-resolution refusals) and where it
// was routed (for the server log).
type chatCompletionErrorScope struct {
	requestedModel string
	appID          string
	provider       string
	model          string
}

// classifyChatCompletionError maps every chat-completion failure to its
// answer. Provider failures keep the provider's verdict where it is the
// caller's to act on (4xx, 429) and become a gateway error where it is not
// (the provider's 5xx, a transport failure, or the provider refusing Helix's
// own credentials). Everything else is a Helix fault and a generic 500.
func classifyChatCompletionError(err error, scope chatCompletionErrorScope) chatCompletionFailure {
	var providerErr *oai.ProviderError
	var quotaErr *controller.InferenceQuotaExceededError
	switch {
	case errors.As(err, &providerErr):
		return classifyProviderFailure(providerErr, scope)
	case isModelResolutionError(err):
		return chatCompletionFailure{status: http.StatusBadRequest, body: chatCompletionErrorBody{
			Message: modelResolutionMessage(scope),
			Type:    errorTypeInvalidRequest,
			Code:    "model_not_found",
		}}
	case errors.Is(err, controller.ErrRAGSourceNotFound):
		return chatCompletionFailure{status: http.StatusNotFound, body: chatCompletionErrorBody{
			Message: "rag_source_id not found",
			Type:    errorTypeInvalidRequest,
			Code:    "rag_source_not_found",
		}}
	case errors.Is(err, controller.ErrInsufficientBalance):
		return chatCompletionFailure{status: http.StatusPaymentRequired, body: chatCompletionErrorBody{
			Message: "insufficient balance; top up the wallet to continue",
			Type:    errorTypeInsufficient,
			Code:    "insufficient_balance",
		}}
	case errors.As(err, &quotaErr):
		return chatCompletionFailure{status: http.StatusPaymentRequired, body: chatCompletionErrorBody{
			Message: quotaErr.Error(),
			Type:    errorTypeInsufficient,
			Code:    "insufficient_quota",
		}}
	default:
		return chatCompletionFailure{status: http.StatusInternalServerError, body: chatCompletionErrorBody{
			Message: "the chat completion failed with an internal error",
			Type:    errorTypeServer,
		}}
	}
}

func modelResolutionMessage(scope chatCompletionErrorScope) string {
	switch {
	case scope.requestedModel != "":
		return fmt.Sprintf("model %q is not available; prefix the model with its provider (e.g. \"openai/%s\") or supply an app_id", scope.requestedModel, scope.requestedModel)
	case scope.appID != "":
		return "the agent's configured model is not available; check the model and provider configured for this agent, or set model"
	default:
		return "no model was specified and no default model is available; set model or supply an app_id"
	}
}

// classifyProviderFailure maps an error from the model provider. It also
// classifies a failure read from an established stream, whose status can no
// longer be sent but whose body still can.
func classifyProviderFailure(err error, scope chatCompletionErrorScope) chatCompletionFailure {
	status := oai.ProviderStatusCode(err)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// Helix's credentials for the provider are wrong; the caller's are not.
		return gatewayFailure(http.StatusBadGateway, "the model provider rejected Helix's credentials; an administrator must check the provider configuration")
	case status == http.StatusTooManyRequests:
		failure := providerRefusal(err, status, errorTypeRateLimit, scope)
		var providerErr *oai.ProviderError
		if errors.As(err, &providerErr) {
			failure.header = providerErr.Header
		}
		return failure
	case status >= 400 && status < 500:
		return providerRefusal(err, status, errorTypeInvalidRequest, scope)
	case status >= 500:
		return gatewayFailure(http.StatusBadGateway, "the model provider failed to process the request")
	case status > 0:
		return gatewayFailure(http.StatusBadGateway, "the model provider returned an unexpected response")
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		// An error event inside an established stream carries no status.
		return providerRefusal(err, http.StatusBadGateway, errorTypeUpstream, scope)
	}
	if isTimeoutError(err) {
		return gatewayFailure(http.StatusGatewayTimeout, "timed out waiting for the model provider")
	}
	return gatewayFailure(http.StatusBadGateway, "the request to the model provider failed")
}

func gatewayFailure(status int, message string) chatCompletionFailure {
	return chatCompletionFailure{status: status, body: chatCompletionErrorBody{Message: message, Type: errorTypeUpstream}}
}

// providerRefusal passes the provider's verdict on: its status, and its
// type, code and param when the body was OpenAI-shaped, with the message
// sanitized. A body that was not OpenAI-shaped (an HTML page from a proxy)
// is replaced by the status text.
func providerRefusal(err error, status int, defaultType string, scope chatCompletionErrorScope) chatCompletionFailure {
	body := chatCompletionErrorBody{
		Message: fmt.Sprintf("the model provider refused the request: %s", http.StatusText(status)),
		Type:    defaultType,
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		models := []string{scope.requestedModel, scope.model}
		if msg := sanitizeProviderMessage(apiErr.Message, models...); msg != "" {
			body.Message = msg
		}
		if t := sanitizeProviderMessage(apiErr.Type); t != "" {
			body.Type = t
		}
		if code, ok := apiErr.Code.(string); ok {
			body.Code = sanitizeProviderMessage(code)
		} else {
			body.Code = apiErr.Code
		}
		if apiErr.Param != nil {
			param := sanitizeProviderMessage(*apiErr.Param)
			body.Param = &param
		}
	}
	return chatCompletionFailure{status: status, body: body}
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// answerChatCompletionFailure answers a failed chat completion, unless the
// caller has gone away: then the failure is the disconnect itself (the
// provider call was cancelled with the request), there is no one to answer,
// and it is not a provider fault. Usage already consumed is recorded by the
// logging middleware either way.
func answerChatCompletionFailure(rw http.ResponseWriter, r *http.Request, err error, scope chatCompletionErrorScope, streamStarted bool) {
	if r.Context().Err() != nil {
		log.Info().
			Str("provider", scope.provider).
			Str("model", scope.model).
			Bool("stream_started", streamStarted).
			Msg("chat completion client disconnected")
		return
	}
	if streamStarted {
		writeChatCompletionStreamError(rw, err, scope)
		return
	}
	writeChatCompletionError(rw, err, scope)
}

// writeChatCompletionError answers a chat completion that failed before any
// of the response was sent.
func writeChatCompletionError(rw http.ResponseWriter, err error, scope chatCompletionErrorScope) {
	failure := classifyChatCompletionError(err, scope)
	logChatCompletionFailure(err, failure, scope)
	for name, values := range failure.header {
		rw.Header()[name] = values
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(failure.status)
	if encodeErr := json.NewEncoder(rw).Encode(chatCompletionErrorResponse{Error: failure.body}); encodeErr != nil {
		log.Error().Err(encodeErr).Msg("failed to write chat completion error")
	}
}

// writeChatCompletionStreamError ends a stream that already sent chunks, so
// its 200 status is gone. Like OpenAI, it sends the error object as a final
// `data: {"error": …}` event and no [DONE]; the OpenAI SDKs (and go-openai)
// raise that event as an API error.
func writeChatCompletionStreamError(rw http.ResponseWriter, err error, scope chatCompletionErrorScope) {
	failure := classifyProviderFailure(err, scope)
	logChatCompletionFailure(err, failure, scope)
	bts, marshalErr := json.Marshal(chatCompletionErrorResponse{Error: failure.body})
	if marshalErr != nil {
		log.Error().Err(marshalErr).Msg("failed to encode chat completion stream error")
		return
	}
	if writeErr := writeChunk(rw, bts); writeErr != nil {
		log.Error().Err(writeErr).Msg("failed to write chat completion stream error")
	}
}

func logChatCompletionFailure(err error, failure chatCompletionFailure, scope chatCompletionErrorScope) {
	event := log.Warn()
	if failure.status >= http.StatusInternalServerError {
		event = log.Error()
	}
	event.
		Str("error", redactCredentials(err.Error())).
		Str("provider", scope.provider).
		Str("model", scope.model).
		Int("upstream_status", oai.ProviderStatusCode(err)).
		Int("status", failure.status).
		Msg("chat completion failed")
}

const maxProviderMessageBytes = 512

var (
	urlPattern      = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	ipv4Pattern     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b`)
	ipv6Pattern     = regexp.MustCompile(`(?i)\[?\b(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{0,4}\]?(?::\d{1,5})?`)
	hostPortPattern = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9.-]*:\d{2,5}\b`)
	// Helix's own cluster hosts are named node01, node06, …
	internalNodePattern = regexp.MustCompile(`(?i)\bnode\d{1,3}\b`)
	// Dotted names under a public or internal suffix. Restricting the suffix
	// keeps parameter paths such as "messages.0.content" readable.
	hostnamePattern = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:com|net|org|io|ai|dev|app|cloud|co|uk|us|eu|xyz|sh|me|internal|local|localhost|lan|corp|svc|cluster|intra|home)\b`)

	credentialAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|secret|password|authorization|key)(\s*[:=]\s*)"?[^\s"',;]+`)
	bearerPattern               = regexp.MustCompile(`(?i)\bbearer\s+\S+`)
	prefixedKeyPattern          = regexp.MustCompile(`\b(?:sk|pk|rk|hl)-[A-Za-z0-9_*-]{6,}|\b(?:ghp|gho|ghs|ghu|github_pat|xoxb|xoxp|hf|gsk|xai)_[A-Za-z0-9_]{6,}|\bAIza[0-9A-Za-z_-]{16,}`)
	jwtPattern                  = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	// An opaque run of 24+ credential characters containing a digit: an
	// unprefixed key or token. Words and snake_case names have no digits.
	opaqueTokenPattern = regexp.MustCompile(`\b[A-Za-z0-9_-]*[0-9][A-Za-z0-9_-]*\b`)
)

// redactCredentials masks anything that looks like a key or token, keeping
// the rest (including URLs and hosts) for the server log.
func redactCredentials(s string) string {
	s = jwtPattern.ReplaceAllString(s, auditlog.RedactedValue)
	s = bearerPattern.ReplaceAllString(s, "Bearer "+auditlog.RedactedValue)
	s = credentialAssignmentPattern.ReplaceAllString(s, "${1}${2}"+auditlog.RedactedValue)
	s = prefixedKeyPattern.ReplaceAllString(s, auditlog.RedactedValue)
	return opaqueTokenPattern.ReplaceAllStringFunc(s, func(word string) string {
		if len(word) >= 24 {
			return auditlog.RedactedValue
		}
		return word
	})
}

// sanitizeProviderMessage makes provider error text safe to return to the
// caller: credentials, URLs, IP addresses and hostnames are replaced, and the
// result is capped. The caller's own model names (which can look like opaque
// tokens) are kept verbatim.
func sanitizeProviderMessage(s string, models ...string) string {
	for _, m := range models {
		if m == "" || !strings.Contains(s, m) {
			continue
		}
		parts := strings.Split(s, m)
		for i, part := range parts {
			parts[i] = sanitizeProviderText(part)
		}
		return capProviderMessage(strings.Join(parts, m))
	}
	return capProviderMessage(sanitizeProviderText(s))
}

func sanitizeProviderText(s string) string {
	s = urlPattern.ReplaceAllString(s, "[url]")
	s = redactCredentials(s)
	s = ipv4Pattern.ReplaceAllString(s, "[address]")
	s = ipv6Pattern.ReplaceAllString(s, "[address]")
	s = hostPortPattern.ReplaceAllString(s, "[host]")
	s = hostnamePattern.ReplaceAllString(s, "[host]")
	return internalNodePattern.ReplaceAllString(s, "[host]")
}

func capProviderMessage(s string) string {
	s = strings.TrimSpace(s)
	if truncated, cut := auditlog.Truncate(s, maxProviderMessageBytes); cut {
		return truncated + "…"
	}
	return s
}
