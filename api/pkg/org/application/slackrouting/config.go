package slackrouting

import "encoding/json"

// RouterConfig is the Slack auto-router's Processor.Config. The filter Kind
// ignores it entirely (its predicates live on the Outputs); only the
// thread-follow arm reads it. Kept here so router creation and the follower
// share one shape.
type RouterConfig struct {
	// ThreadFollow, when true, routes every later message in a thread to the
	// Workers already participating in it — not just the Workers named in
	// the message. On by default (see DefaultConfig): it matches what Slack
	// users expect — once you're in a thread you keep getting replies.
	ThreadFollow bool `json:"thread_follow"`
}

// DefaultConfig is the Config a freshly-created auto-router carries:
// thread-follow ON, matching Slack's "everyone in the thread is notified"
// expectation. Operators can turn it off per-router in the UI.
func DefaultConfig() json.RawMessage {
	b, _ := json.Marshal(RouterConfig{ThreadFollow: true})
	return b
}

// ThreadFollowEnabled reports whether a router Config opts into thread
// participation. A malformed or empty blob reads as off.
func ThreadFollowEnabled(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var c RouterConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return false
	}
	return c.ThreadFollow
}
