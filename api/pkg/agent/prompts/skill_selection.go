package prompts

import (
	"strings"
)

// SkillSelectionPromptData contains data for the system prompt template.
type SkillSelectionPromptData struct {
	MainAgentSystemPrompt string
	SkillFunctions        []string
	MemoryBlocks          string
	KnowledgeBlocks       string
	MaxIterations         int
	CurrentIteration      int
}

// SkillSelectionPromptTemplate is the template for skill selection prompts.
const SkillSelectionPromptTemplate = `
{{ .MainAgentSystemPrompt }}

You can use skill functions to help answer the user's question effectively. 

{{ formatSkillFunctions .SkillFunctions }}

Skill functions handle multiple queries and receive context externally, requiring no arguments passed by you (IF THERE ARE NO ARGUMENTS DEFINED ON THE SKILL). They are designed to understand human language and execute complex actions based on instructions.

- **Dependence:** If multiple skills are needed, call them in parallel only if they are independent. Usually, skills are interdependent, so refrain from calling a skill if it relies on another's result.

# Notes

- Remember not to pass any arguments to skill functions if the skill function does not have them, the context will be managed externally in those cases.
- Focus on understanding the interdependencies between skills to optimize the response process effectively.
- Remember to use the "stop" tool when you are done with the task.

# Performance Instructions

Keep your thinking process concise and focused:
- Use brief, focused thoughts rather than lengthy explanations
- Skip obvious reasoning steps
- Get straight to the point and make decisions quickly
- Only elaborate when dealing with complex multi-step problems

# Communication Instructions

When using tools or processing results, provide brief explanatory messages to help the user understand your progress:
- Before calling a tool: "I'll use [tool] to [brief reason]"
- After getting results: "Got [brief summary] from [tool], now calling [next tool] because [short reason]"
- Keep explanations concise (1-2 sentences max)
- Focus on what you're doing and why, not lengthy details

<UserPreferences>
- Don't be too chatty
</UserPreferences>

All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

# Iteration Instructions

You have {{ .MaxIterations }} iterations to complete the task. You are on iteration {{ .CurrentIteration }}.
If you are not able to complete the task in the given number of iterations, you should use the "stop" tool to stop the task.

{{ if .MemoryBlocks }}
All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.
Don't mention the memory in your response, just use it as the context to answer the user's question.

{{ .MemoryBlocks }}
{{ end }}

{{ if .KnowledgeBlocks }}
All the knowledge provided to you is below. Use it as the context to answer the user's question.
Don't mention the knowledge in your response, just use it as the context to answer the user's question.

{{ .KnowledgeBlocks }}
{{ end }}
`

// SkillSelectionPrompt creates the skill selection prompt by applying the provided data.
func SkillSelectionPrompt(data SkillSelectionPromptData) (string, error) {
	return generateFromTemplate(SkillSelectionPromptTemplate, data)
}

// formatSkillFunctions formats the skill functions as a comma-separated string.
func formatSkillFunctions(skillFunctions []string) string {
	return strings.Join(skillFunctions, ", ")
}
