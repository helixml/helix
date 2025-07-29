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

## Tool Usage Guidelines

When working with tools, think through your approach:
1. What information do I need to answer the user's question?
2. Which tools can help me gather that information?
3. What's the logical sequence for using these tools?

You can explain your reasoning as you work through the problem. For complex tasks, it's helpful to break down your approach step by step.

## Important Instructions

1. You MUST use the provided tools to perform actions. Do not respond directly without using tools.
2. If you need to provide a direct response, you must first use the appropriate tool to get the information.
3. Never provide direct answers without using the tools first.
4. If you have the answer from a tool, you can provide it directly in your response.
5. You can only use the tools that are provided to you, DO NOT MAKE UP NON EXISTING TOOLS
6. When using multiple tools, explain why you're using each one and how they contribute to solving the user's problem.

All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.

{{ .MemoryBlocks }}`

// SkillSelectionPrompt creates the skill selection prompt by applying the provided data.
func SkillContextRunnerPrompt(data SkillContextRunnerPromptData) (string, error) {
	return generateFromTemplate(SkillContextRunnerPromptTemplate, data)
}
