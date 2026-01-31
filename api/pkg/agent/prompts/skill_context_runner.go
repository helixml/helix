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

IMPORTANT INSTRUCTIONS:
1. You MUST use the provided tools to perform actions. Do not respond directly without using tools.
2. If you need to provide a direct response, you must first use the appropriate tool to get the information.
3. Never provide direct answers without using the tools first.
4. If you have the answer from a tool, you can provide it directly in your response.
5. You can only use the tools that are provided to you, DO NOT MAKE UP NON EXISTING TOOLS

PERFORMANCE:
- Keep thinking concise and focused - get straight to tool usage
- Skip lengthy explanations, make decisions quickly
- Only elaborate for complex multi-step reasoning

COMMUNICATION:
- Provide brief explanatory messages about your actions: "Using [tool] to [reason]" or "Got [result], now [next action]"
- Keep explanations short (1-2 sentences) but informative
- Help users understand your progress without lengthy details
- If you are providing information in a table, write valid GFM markdown, always leave a blank line before and after the table

{{ if .MemoryBlocks }}
All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}
{{ end }}
`

// SkillSelectionPrompt creates the skill selection prompt by applying the provided data.
func SkillContextRunnerPrompt(data SkillContextRunnerPromptData) (string, error) {
	return generateFromTemplate(SkillContextRunnerPromptTemplate, data)
}
