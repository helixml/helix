package prompts

import (
	"strings"
)

// SkillSelectionPromptData contains data for the system prompt template.
type SkillSelectionPromptData struct {
	MainAgentSystemPrompt string
	SkillFunctions        []string
	MemoryBlocks          string
}

// SkillSelectionPromptTemplate is the template for skill selection prompts.
const SkillSelectionPromptTemplate = `
{{ .MainAgentSystemPrompt }}

You can use skill functions to help answer the user's question effectively. 

{{ formatSkillFunctions .SkillFunctions }}

Skill functions handle multiple queries and receive context externally, requiring no arguments passed by you. They are designed to understand human language and execute complex actions based on instructions.

- **Dependence:** If multiple skills are needed, call them in parallel only if they are independent. Usually, skills are interdependent, so refrain from calling a skill if it relies on another's result.

# Notes

- Remember not to pass any arguments to skill functions, as the context is managed externally.
- Focus on understanding the interdependencies between skills to optimize the response process effectively.
- Remember to use the "stop" tool when you are done with the task.

<UserPreferences>
- Don't be too chatty
</UserPreferences>

All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}`

// SkillSelectionPrompt creates the skill selection prompt by applying the provided data.
func SkillSelectionPrompt(data SkillSelectionPromptData) (string, error) {
	return generateFromTemplate(SkillSelectionPromptTemplate, data)
}

// formatSkillFunctions formats the skill functions as a comma-separated string.
func formatSkillFunctions(skillFunctions []string) string {
	return strings.Join(skillFunctions, ", ")
}
