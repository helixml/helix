package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// thinkingRetryTransport wraps an http.RoundTripper to recover from a
// schema mismatch on the Anthropic `thinking.type` field when going through
// Vertex AI.
//
// Vertex's global load balancer fronts inconsistent backend pods. Some pods
// only accept the older schema (`enabled` / `disabled`) and reject `adaptive`
// with:
//
//	"thinking: Input tag 'adaptive' found using 'type' does not match
//	 any of the expected tags: 'disabled', 'enabled'"
//
// Other pods only accept `adaptive` for newer models like claude-opus-4-7
// and reject `enabled` with:
//
//	`"thinking.type.enabled" is not supported for this model.
//	 Use "thinking.type.adaptive" and "output_config.effort" to control
//	 thinking behavior.`
//
// We can't predict which pod the LB will pick. Instead, on a 400 that matches
// either pattern, swap the thinking.type and retry once.
type thinkingRetryTransport struct {
	base http.RoundTripper
}

func (t *thinkingRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Skip non-POST or anything without a body — only Anthropic /v1/messages
	// requests carry a thinking field.
	if req.Method != http.MethodPost || req.Body == nil {
		return t.base.RoundTrip(req)
	}

	origBody, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}

	// Upfront mutation: if the request has thinking.type=adaptive but no
	// top-level output_config.effort, inject medium effort *before* sending.
	//
	// Why this matters: claude-agent-acp sends adaptive directly for opus-4-7
	// (which Vertex requires), so the swap-from-enabled retry path below
	// never fires. Without this upfront injection the model runs at its
	// implicit minimum effort, producing empty thinking summaries — visible
	// to the user as `<thinking></thinking>` blocks with no content.
	// The Vertex error message we parse on the retry path explicitly says
	// to set both fields together; this just applies the same rule before
	// we even see a 400. ensureOutputConfigEffort respects any caller-set
	// effort and preserves other output_config fields.
	if mutatedBody, didMutate := ensureAdaptiveEffort(origBody, "medium"); didMutate {
		log.Info().
			Int("orig_len", len(origBody)).
			Int("new_len", len(mutatedBody)).
			Msg("injecting output_config.effort=medium for adaptive-thinking request (otherwise summary is empty)")
		origBody = mutatedBody
		req.ContentLength = int64(len(origBody))
		req.Header.Del("Content-Length")
	}

	req.Body = io.NopCloser(bytes.NewReader(origBody))
	// Repopulate GetBody too in case the underlying transport wants to retry on
	// a transport-level redirect.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(origBody)), nil
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		return resp, err
	}

	// Buffer the response body so we can inspect it. If it's not a thinking
	// schema mismatch, we replay the original body to the caller unchanged.
	respBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}

	altType := detectThinkingTypeMismatch(respBody)
	if altType == "" {
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		resp.ContentLength = int64(len(respBody))
		return resp, nil
	}

	newBody, ok := swapThinkingType(origBody, altType)
	if !ok {
		// Body didn't have a thinking field, or wasn't valid JSON — surface the
		// original 400 so the caller sees the actual error.
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		resp.ContentLength = int64(len(respBody))
		return resp, nil
	}

	log.Info().
		Str("alt_type", altType).
		Int("orig_len", len(origBody)).
		Int("new_len", len(newBody)).
		Msg("retrying Anthropic request with alternate thinking.type after Vertex 400")

	retryReq := req.Clone(req.Context())
	retryReq.Body = io.NopCloser(bytes.NewReader(newBody))
	retryReq.ContentLength = int64(len(newBody))
	retryReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}
	// Drop any precomputed Content-Length header so the transport rewrites it
	// from retryReq.ContentLength.
	retryReq.Header.Del("Content-Length")

	return t.base.RoundTrip(retryReq)
}

// detectThinkingTypeMismatch inspects a 400 response body for either of the
// two known Vertex schema-mismatch errors and returns the alternate type to
// retry with ("enabled" or "adaptive"), or "" if the body is not a thinking
// schema mismatch.
func detectThinkingTypeMismatch(body []byte) string {
	s := string(body)
	switch {
	case strings.Contains(s, "Input tag 'adaptive'"):
		// Pod doesn't know "adaptive" yet. Retry with "enabled".
		return "enabled"
	case strings.Contains(s, "thinking.type.enabled") && strings.Contains(s, "adaptive"):
		// Pod requires "adaptive" for this model. Retry with "adaptive".
		return "adaptive"
	}
	return ""
}

// swapThinkingType rewrites the request body's thinking.type to newType.
//
// When switching to "enabled": injects budget_tokens=4096 if missing and
// removes the adaptive-only "display" field.
//
// When switching to "adaptive": removes budget_tokens (adaptive rejects it)
// and ensures top-level output_config.effort is set. The Vertex rejection
// that triggers this swap explicitly says "Use thinking.type.adaptive and
// output_config.effort to control thinking behavior" — the two fields work
// as a pair. Without effort, adaptive runs at the model's implicit minimum,
// which produces an empty thinking summary visible to the client (renders
// as `<thinking></thinking>` with no content). We default to "medium",
// which is the right trade-off between latency and useful summary depth
// for the Helix spec-task workload. Caller can pre-set
// output_config.effort to override.
//
// Returns (newBody, true) on success, (nil, false) if the body is not valid
// JSON or has no thinking field.
func swapThinkingType(body []byte, newType string) ([]byte, bool) {
	var bodyMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, false
	}
	thinkingRaw, ok := bodyMap["thinking"]
	if !ok {
		return nil, false
	}
	var thinking map[string]json.RawMessage
	if err := json.Unmarshal(thinkingRaw, &thinking); err != nil {
		return nil, false
	}

	thinking["type"], _ = json.Marshal(newType)
	switch newType {
	case "enabled":
		if _, has := thinking["budget_tokens"]; !has {
			thinking["budget_tokens"], _ = json.Marshal(4096)
		}
		delete(thinking, "display")
	case "adaptive":
		delete(thinking, "budget_tokens")
		if !ensureOutputConfigEffort(bodyMap, "medium") {
			return nil, false
		}
	}

	newThinking, err := json.Marshal(thinking)
	if err != nil {
		return nil, false
	}
	bodyMap["thinking"] = newThinking

	newBody, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, false
	}
	return newBody, true
}

// ensureAdaptiveEffort inspects body and, if it has thinking.type=adaptive
// without a top-level output_config.effort, injects effort. Returns
// (newBody, true) when a mutation happened, (nil, false) otherwise — including
// when the body has no thinking field, isn't adaptive, already has effort,
// or isn't valid JSON. Callers should fall through to the unmutated body
// in the false case rather than treating it as an error.
func ensureAdaptiveEffort(body []byte, effort string) ([]byte, bool) {
	var bodyMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, false
	}
	thinkingRaw, ok := bodyMap["thinking"]
	if !ok {
		return nil, false
	}
	var thinking map[string]json.RawMessage
	if err := json.Unmarshal(thinkingRaw, &thinking); err != nil {
		return nil, false
	}
	typeRaw, ok := thinking["type"]
	if !ok {
		return nil, false
	}
	var typeStr string
	if err := json.Unmarshal(typeRaw, &typeStr); err != nil {
		return nil, false
	}
	if typeStr != "adaptive" {
		return nil, false
	}
	// Already has output_config.effort? Don't touch it.
	if raw, ok := bodyMap["output_config"]; ok {
		var outputConfig map[string]json.RawMessage
		if err := json.Unmarshal(raw, &outputConfig); err == nil {
			if _, has := outputConfig["effort"]; has {
				return nil, false
			}
		}
	}
	if !ensureOutputConfigEffort(bodyMap, effort) {
		return nil, false
	}
	newBody, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, false
	}
	return newBody, true
}

// ensureOutputConfigEffort sets bodyMap["output_config"]["effort"] = effort
// if not already set. Preserves any other fields under output_config.
// Returns false on JSON malformedness.
func ensureOutputConfigEffort(bodyMap map[string]json.RawMessage, effort string) bool {
	outputConfig := map[string]json.RawMessage{}
	if raw, ok := bodyMap["output_config"]; ok {
		if err := json.Unmarshal(raw, &outputConfig); err != nil {
			return false
		}
	}
	if _, has := outputConfig["effort"]; has {
		return true
	}
	effortRaw, err := json.Marshal(effort)
	if err != nil {
		return false
	}
	outputConfig["effort"] = effortRaw
	newOutputConfig, err := json.Marshal(outputConfig)
	if err != nil {
		return false
	}
	bodyMap["output_config"] = newOutputConfig
	return true
}
