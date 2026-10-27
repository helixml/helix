package types

// CodeAgentModelOption is one model a coding-agent harness can run with its
// vendor's own credentials (a Claude or ChatGPT subscription).
type CodeAgentModelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Current marks a current-generation model. Pickers list current models
	// first and group the rest as legacy.
	Current bool `json:"current"`
}

// CodeAgentModels is the single source of truth for which vendor models each
// harness offers, best first. The first entry is the harness's default model,
// so moving a newer model to the top moves every default with it:
//   - new tasks and agents created without an explicit model,
//   - the UI pickers, which read this list from /api/v1/config,
//   - HelixOS, which moves its long-lived bots onto the first entry.
//
// Edit this list and nothing else when a vendor ships a new model.
var CodeAgentModels = map[CodeAgentRuntime][]CodeAgentModelOption{
	CodeAgentRuntimeClaudeCode: {
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5 (1M context)", Current: true},
		{ID: "claude-opus-5", Name: "Claude Opus 5 (1M context)", Current: true},
		{ID: "claude-fable-5", Name: "Claude Fable 5 (1M context)", Current: true},
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8 (1M context)"},
		// Sonnet and Haiku deliberately use Claude Code's rolling aliases.
		{ID: "sonnet", Name: "Claude Sonnet (latest)"},
		{ID: "haiku", Name: "Claude Haiku (latest)"},
	},
	CodeAgentRuntimeCodexCLI: {
		{ID: "gpt-6-sol", Name: "GPT-6 Sol", Current: true},
		{ID: "gpt-6-astra", Name: "GPT-6 Astra", Current: true},
		{ID: "gpt-6-luna", Name: "GPT-6 Luna", Current: true},
		{ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Current: true},
		{ID: "gpt-5.6-terra", Name: "GPT-5.6 Terra", Current: true},
		{ID: "gpt-5.6-luna", Name: "GPT-5.6 Luna", Current: true},
		{ID: "gpt-5.5", Name: "GPT-5.5"},
		{ID: "gpt-5.4", Name: "GPT-5.4"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
		{ID: "gpt-5.3-codex-spark", Name: "GPT-5.3 Codex Spark"},
	},
}

// DefaultCodeAgentModel is the model a harness runs when none is set: the top
// of its list. Empty for a harness without a vendor model list.
func DefaultCodeAgentModel(runtime CodeAgentRuntime) string {
	if models := CodeAgentModels[runtime]; len(models) > 0 {
		return models[0].ID
	}
	return ""
}
