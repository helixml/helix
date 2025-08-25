package prompts

// NoSkillsPromptData contains data for the system prompt template when no skills are available.
type NoSkillsPromptData struct {
	MainAgentSystemPrompt string
	MemoryBlocks          string
	KnowledgeBlocks       string
}

// NoSkillsPromptTemplate is the template for prompts when no skills are available.
const NoSkillsPromptTemplate = `
{{ .MainAgentSystemPrompt }}

{{ if .MemoryBlocks }}
All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}
{{ end }}

{{ if .KnowledgeBlocks }}
All the knowledge provided to you is below. Use it as the context to answer the user's question.

{{ .KnowledgeBlocks }}
{{ end }}
`

// NoSkillsPrompt creates a prompt for when no skills are available by applying the provided data.
func NoSkillsPrompt(data NoSkillsPromptData) (string, error) {
	return generateFromTemplate(NoSkillsPromptTemplate, data)
}
