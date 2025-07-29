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

You can use skill functions to help answer the user's question effectively. When deciding which skills to use, think through your approach step by step.

{{ formatSkillFunctions .SkillFunctions }}

Skill functions handle multiple queries and receive context externally, requiring no arguments passed by you. They are designed to understand human language and execute complex actions based on instructions.

## Decision Process

When choosing skills, consider:
1. What information or capabilities do I need to answer the user's question?
2. Which skills provide those capabilities?
3. Are there dependencies between skills that require sequential execution?
4. Can any skills be run in parallel?

If you need to explain your reasoning, do so naturally as part of your response. For complex decisions, walk through your thought process.

## Guidelines

- **Dependence:** If multiple skills are needed, call them in parallel only if they are independent. Usually, skills are interdependent, so refrain from calling a skill if it relies on another's result.
- Remember not to pass any arguments to skill functions, as the context is managed externally.
- Focus on understanding the interdependencies between skills to optimize the response process effectively.
- Remember to use the "stop" tool when you are done with the task.

<UserPreferences>
- Don't be too chatty
- Show your reasoning when making complex decisions
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
