package slack

import (
	"context"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Candidate is one subscribed Worker the Router may activate, paired
// with the text the Router scores against (its identity + role text).
type Candidate struct {
	WorkerID orgchart.WorkerID
	Identity string
}

// Inbound is the routing context: the new message, prior thread
// messages (oldest-first), and the channel's subscriber set. A Router
// chooses a subset of Subscribers — it never invents a target outside
// it.
type Inbound struct {
	OrgID       string
	Stream      streaming.Stream
	Message     streaming.Message
	Thread      []streaming.Message
	Subscribers []Candidate
}

// Router narrows the subscriber set to the Worker(s) an inbound message
// should activate (§9.4 / OQ-1). Personas can't be @mentioned, so when
// several Workers share a channel something has to pick — but that
// choice is an org-graph concern, pluggable per org. The default is to
// not choose at all (broadcast).
type Router interface {
	Route(ctx context.Context, in Inbound) ([]orgchart.WorkerID, error)
}

// BroadcastRouter returns every subscriber (FR-25). Helix decides
// nothing; disambiguation is left to the org graph (e.g. a routing
// Worker reading the channel). This is the default.
type BroadcastRouter struct{}

// Route returns all subscriber WorkerIDs.
func (BroadcastRouter) Route(_ context.Context, in Inbound) ([]orgchart.WorkerID, error) {
	out := make([]orgchart.WorkerID, 0, len(in.Subscribers))
	for _, c := range in.Subscribers {
		out = append(out, c.WorkerID)
	}
	return out, nil
}

// FuzzyRouter scores each candidate by keyword overlap of the
// message + thread text against the Worker's identity/role text and
// returns the best match. Code-only — no LLM, no network. It never
// drops to zero: on a tie among the top scorers, or when no candidate
// scores above zero (low confidence), it returns all tied / all
// subscribers, so a message is never silently delivered to nobody.
type FuzzyRouter struct{}

// Route picks the highest-overlap candidate(s).
func (FuzzyRouter) Route(_ context.Context, in Inbound) ([]orgchart.WorkerID, error) {
	if len(in.Subscribers) == 0 {
		return nil, nil
	}
	// Build the haystack of message words (new message + thread).
	var sb strings.Builder
	sb.WriteString(in.Message.Subject)
	sb.WriteByte(' ')
	sb.WriteString(in.Message.Body)
	for _, m := range in.Thread {
		sb.WriteByte(' ')
		sb.WriteString(m.Subject)
		sb.WriteByte(' ')
		sb.WriteString(m.Body)
	}
	msgTokens := tokenSet(sb.String())

	best := 0
	scores := make([]int, len(in.Subscribers))
	for i, c := range in.Subscribers {
		score := overlap(msgTokens, tokenSet(c.Identity))
		scores[i] = score
		if score > best {
			best = score
		}
	}
	// Low confidence: nobody matched → broadcast (never drop).
	if best == 0 {
		return BroadcastRouter{}.Route(context.Background(), in)
	}
	var out []orgchart.WorkerID
	for i, c := range in.Subscribers {
		if scores[i] == best {
			out = append(out, c.WorkerID)
		}
	}
	return out, nil
}

// overlap counts how many message tokens appear in the identity tokens.
func overlap(msg, identity map[string]struct{}) int {
	n := 0
	for tok := range msg {
		if _, ok := identity[tok]; ok {
			n++
		}
	}
	return n
}

// stopWords are common words excluded from fuzzy matching so they don't
// dominate the overlap score.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {},
	"in": {}, "on": {}, "for": {}, "is": {}, "are": {}, "i": {}, "you": {},
	"my": {}, "me": {}, "we": {}, "it": {}, "this": {}, "that": {}, "can": {},
	"please": {}, "was": {}, "with": {}, "at": {}, "be": {}, "have": {},
}

// tokenSet lowercases, splits on non-alphanumeric runs, drops stop
// words and 1-char tokens, naively singularises (so "invoices" matches
// "invoice"), and returns the distinct set.
func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(f) < 2 {
			continue
		}
		if _, skip := stopWords[f]; skip {
			continue
		}
		out[stem(f)] = struct{}{}
	}
	return out
}

// stem applies a deliberately crude singularisation: trim a trailing
// "s" from tokens longer than three characters ("refunds" → "refund",
// "payments" → "payment"). Good enough to align plural/singular forms
// for keyword overlap without pulling in a real stemmer.
func stem(tok string) string {
	if len(tok) > 3 && strings.HasSuffix(tok, "s") {
		return tok[:len(tok)-1]
	}
	return tok
}

// routerRegistry maps the per-org `slack.router` config value to a
// Router. Adding an implementation (e.g. an LLMRouter) is a new entry
// here plus the type — no edits to the ingest. Unknown/unset names
// resolve to broadcast.
var routerRegistry = map[string]Router{
	"broadcast": BroadcastRouter{},
	"fuzzy":     FuzzyRouter{},
}

// RouterFor resolves a `slack.router` config value to a Router,
// defaulting to BroadcastRouter for unknown or empty names (§9.4).
func RouterFor(name string) Router {
	if r, ok := routerRegistry[name]; ok {
		return r
	}
	return BroadcastRouter{}
}
