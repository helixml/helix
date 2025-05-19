package prompts

// SystemPromptData contains data for the system prompt template.
type SkillContextRunnerPromptData struct {
	MainAgentSystemPrompt string
	SkillSystemPrompt     string
	MemoryBlocks          string
}

// SkillSelectionPromptTemplate is the template for skill selection prompts.
const SkillContextRunnerPromptTemplate = `
{{ .MainAgentSystemPrompt }}

{{ .SkillSystemPrompt }}

All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}`

// SkillSelectionPrompt creates the skill selection prompt by applying the provided data.
func SkillContextRunnerPrompt(data SkillContextRunnerPromptData) (string, error) {
	return generateFromTemplate(SkillContextRunnerPromptTemplate, data)
}
