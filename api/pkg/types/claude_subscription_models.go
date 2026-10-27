package types

// ClaudeSubscriptionModelOption is one entry in the Claude Code subscription
// model picker.
type ClaudeSubscriptionModelOption struct {
	ID          string
	Name        string
	Description string
}

// ClaudeSubscriptionModels is the Claude Code subscription model picker, best
// first. Its first entry is also the default for any claude_code subscription
// task that does not name a model, so putting a newer model at the top moves
// every new task onto it. Keep the order deliberate.
var ClaudeSubscriptionModels = []ClaudeSubscriptionModelOption{
	{ID: "claude-opus-5-5", Name: "Claude Opus 5.5 (1M context)", Description: "Recommended Opus model with a 1M-token context window"},
	{ID: "claude-opus-5", Name: "Claude Opus 5 (1M context)", Description: "Previous Opus model with a 1M-token context window"},
	{ID: "claude-fable-5", Name: "Claude Fable 5 (1M context)", Description: "Most capable generally available Claude model"},
	{ID: "claude-opus-4-8", Name: "Claude Opus 4.8 (1M context)", Description: "Older Opus model with a 1M-token context window"},
	{ID: "sonnet", Name: "Claude Sonnet (latest)", Description: "Best balance of speed and capability"},
	{ID: "haiku", Name: "Claude Haiku (latest)", Description: "Fastest Claude model"},
}

// DefaultClaudeSubscriptionModel is the model a claude_code subscription task
// runs when none is set: the top of the picker.
func DefaultClaudeSubscriptionModel() string {
	return ClaudeSubscriptionModels[0].ID
}
