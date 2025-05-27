package prompts

// NoSkillsPromptData contains data for the system prompt template when no skills are available.
type NoSkillsPromptData struct {
	MainAgentSystemPrompt string
	MemoryBlocks          string
}

// NoSkillsPromptTemplate is the template for prompts when no skills are available.
const NoSkillsPromptTemplate = `
{{ .MainAgentSystemPrompt }}

All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}`

// NoSkillsPrompt creates a prompt for when no skills are available by applying the provided data.
func NoSkillsPrompt(data NoSkillsPromptData) (string, error) {
	return generateFromTemplate(NoSkillsPromptTemplate, data)
}
