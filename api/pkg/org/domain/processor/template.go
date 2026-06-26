package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	txttemplate "text/template"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// KindTemplate is the flagship v1 Kind: rewrite a Message's body by
// rendering a Go text/template against the Message. Always passes (1
// input → 1 output).
//
// text/template, NOT html/template: the output is plain text consumed
// by an agent, not browser HTML (design decision 10). Revisit if the
// output is ever HTML-rendered.
const KindTemplate Kind = "template"

// templateConfig is the parsed config for KindTemplate.
type templateConfig struct {
	Template string `json:"template"`
}

// templateFuncs is the small, fixed FuncMap available to processor
// templates. No arbitrary user funcs in v1 (design Open Question 4):
// stateless, no access to prior messages or accumulator state.
var templateFuncs = txttemplate.FuncMap{
	"upper":   strings.ToUpper,
	"lower":   strings.ToLower,
	"trunc":   func(n int, s string) string { return runeSafeTruncate(s, n) },
	"default": func(def, val string) string {
		if val == "" {
			return def
		}
		return val
	},
	// String predicates — primarily for the filter kind's Output.Match,
	// but available to template bodies too. Args are (needle, haystack)
	// so they read naturally: `contains "invoice" .Message.subject`.
	"contains":  func(sub, s string) bool { return strings.Contains(s, sub) },
	"hasPrefix": func(pre, s string) bool { return strings.HasPrefix(s, pre) },
	"hasSuffix": func(suf, s string) bool { return strings.HasSuffix(s, suf) },
	// mentions reports whether name appears in s as a whole token,
	// case-insensitively. It is what the Slack auto-router's per-Worker routes
	// match on, so "Sam" fires on "hey Sam" but not on "salmon", and the FULL
	// worker id "w-jokebot" fires on "w-jokebot" but NOT on "w-jokebot-2".
	// Reads as a predicate: `mentions "w-jokebot" .Message.body`.
	"mentions": mentionsWord,
}

// mentionsWord reports a case-insensitive, whole-token match of name in s. An
// empty name never matches (a route with no name is inert rather than matching
// everything). The pattern is cached per distinct name so repeated predicate
// evaluation on a hot Topic doesn't recompile the regexp.
func mentionsWord(name, s string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return wordRegexp(name).MatchString(s)
}

var (
	wordRegexpMu    sync.Mutex
	wordRegexpCache = map[string]*regexp.Regexp{}
)

// wordRegexp matches name as a whole token. We can't use \b: a hyphen is a
// non-word char, so \bw-jokebot\b would treat the dash in "w-jokebot-2" as a
// boundary and over-match. Worker ids ARE hyphenated, so the token alphabet
// includes `-`; the match must therefore be flanked by start/end of string or
// a char that is neither a word char nor a hyphen ([^\w-]). RE2 has no
// lookarounds, so the flanks are matched (and consumed) explicitly — fine for
// a boolean MatchString, which only needs one occurrence.
func wordRegexp(name string) *regexp.Regexp {
	wordRegexpMu.Lock()
	defer wordRegexpMu.Unlock()
	if re, ok := wordRegexpCache[name]; ok {
		return re
	}
	re := regexp.MustCompile(`(?i)(^|[^\w-])` + regexp.QuoteMeta(name) + `([^\w-]|$)`)
	wordRegexpCache[name] = re
	return re
}

// Validate enforces: exactly one output, with an empty Match (a
// transform is unconditional); a non-empty template; and that the
// template parses with the fixed FuncMap.
func (c templateConfig) Validate(out []Output) error {
	if len(out) != 1 {
		return fmt.Errorf("template processor needs exactly 1 output, got %d", len(out))
	}
	if out[0].Match != "" {
		return errors.New("template output must not carry a match predicate (transform is unconditional)")
	}
	if strings.TrimSpace(c.Template) == "" {
		return errors.New("template is empty")
	}
	if _, err := txttemplate.New("proc").Funcs(templateFuncs).Parse(c.Template); err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	return nil
}

// Process renders the template against the input Message and returns a
// single Result whose body is the rendered text. The original Message
// is preserved except Body (replaced) and BodyContentType ("text/plain").
func (c templateConfig) Process(in streaming.Message, out []Output) ([]Result, error) {
	tmpl, err := txttemplate.New("proc").Funcs(templateFuncs).Option("missingkey=zero").Parse(c.Template)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData(in)); err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}
	m := in
	m.Body = buf.String()
	m.BodyContentType = "text/plain"
	return []Result{{TopicID: out[0].TopicID, Message: m}}, nil
}

// template is the Strategy for KindTemplate.
type template struct{}

// ParseConfig decodes the raw blob into a templateConfig. An empty blob
// is a config with an empty template, which Validate rejects.
func (template) ParseConfig(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return templateConfig{}, nil
	}
	var c templateConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse template config: %w", err)
	}
	return c, nil
}

// templateData builds the template's data context. `.Message` is a map
// with every canonical wire key always present (lowercase JSON keys),
// so `{{ .Message.body }}`, `{{ .Message.from }}`, `{{ .Message.subject }}`
// resolve — and a set-but-empty optional field (e.g. an absent subject)
// renders as "" rather than disappearing. Keys not in the map render as
// "" too (missingkey=zero). The Message envelope's `extra` is exposed
// verbatim for transport-specific fields.
//
// `.Event` metadata (ID, TopicID, Source, CreatedAt) is intentionally
// not threaded in v1: Process receives only the Message, keeping the
// Config interface pure. Add it when a template actually needs event
// provenance.
func templateData(m streaming.Message) map[string]any {
	return map[string]any{
		"Message": map[string]any{
			"from":              m.From,
			"to":                m.To,
			"subject":           m.Subject,
			"body":              m.Body,
			"body_content_type": m.BodyContentType,
			"thread_id":         m.ThreadID,
			"in_reply_to":       m.InReplyTo,
			"message_id":        m.MessageID,
			"attachments":       m.Attachments,
			"extra":             m.Extra,
		},
	}
}
