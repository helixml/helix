package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	txttemplate "text/template"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// KindFilter is the selection kind: it publishes the input Message,
// unchanged, to each Output whose predicate matches, and drops it when
// none do. ONE kind serves both a classic keep/drop filter (one output)
// and a content-based router (N outputs) — "each Output is one filter":
// a predicate + a destination. No mutation happens here; compose a
// transform before/after a filter by chaining through topics.
const KindFilter Kind = "filter"

// filterConfig has no fields of its own — the predicates live on each
// Output.Match. The kind just evaluates them.
type filterConfig struct{}

// Validate requires at least one output and that every non-empty
// predicate parses. An empty Output.Match is an unconditional ("default"
// / else) branch.
//
// Predicate language (OQ7, v1 lean): a boolean Go text/template rendered
// against the same Message context as the template kind. It "matches"
// when the rendered string, trimmed, is non-empty and not a falsey token
// ("false"/"0"/"no") — keep-on-match polarity. One engine, consistent
// with the template transform; revisit a structured matcher when the
// rule-builder UI lands.
func (c filterConfig) Validate(out []Output) error {
	if len(out) < 1 {
		return errors.New("filter processor needs at least 1 output")
	}
	for i, o := range out {
		if strings.TrimSpace(o.Match) == "" {
			continue // unconditional branch
		}
		if _, err := txttemplate.New("pred").Funcs(templateFuncs).Parse(o.Match); err != nil {
			return fmt.Errorf("filter output %d predicate: %w", i, err)
		}
	}
	return nil
}

// Process evaluates each output's predicate against the input Message
// and returns a pass-through Result (the message, unchanged) for each
// match. Zero matches = an empty slice = the message is dropped.
func (c filterConfig) Process(_ context.Context, in streaming.Message, out []Output) ([]Result, error) {
	var res []Result
	for i, o := range out {
		match, err := evalPredicate(o.Match, in)
		if err != nil {
			return nil, fmt.Errorf("filter output %d: %w", i, err)
		}
		if match {
			res = append(res, Result{TopicID: o.TopicID, Message: in})
		}
	}
	return res, nil
}

// filter is the Strategy for KindFilter.
type filter struct{}

// ParseConfig ignores the raw blob — filterConfig is empty; predicates
// live on the Outputs.
func (filter) ParseConfig(_ json.RawMessage) (Config, error) {
	return filterConfig{}, nil
}

// evalPredicate renders a boolean text/template predicate against the
// Message. An empty predicate is unconditionally true (default branch).
// Truthiness: trimmed render that is non-empty and not one of the falsey
// tokens "false"/"0"/"no".
func evalPredicate(expr string, m streaming.Message) (bool, error) {
	if strings.TrimSpace(expr) == "" {
		return true, nil
	}
	tmpl, err := txttemplate.New("pred").Funcs(templateFuncs).Option("missingkey=zero").Parse(expr)
	if err != nil {
		return false, fmt.Errorf("parse predicate: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData(m)); err != nil {
		return false, fmt.Errorf("render predicate: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(buf.String())) {
	case "", "false", "0", "no":
		return false, nil
	default:
		return true, nil
	}
}
