package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillContextRunnerPrompt_WithMemoryOnly(t *testing.T) {
	data := SkillContextRunnerPromptData{
		MainAgentSystemPrompt: "You are a helpful AI assistant",
		SkillSystemPrompt:     "You can access databases",
		MemoryBlocks:          "User previously asked about database connections and prefers PostgreSQL",
	}

	prompt, err := SkillContextRunnerPrompt(data)
	require.NoError(t, err)

	// Check that main system prompt is included
	assert.Contains(t, prompt, "You are a helpful AI assistant")

	// Check that skill system prompt is included
	assert.Contains(t, prompt, "You can access databases")

	// Check that memory blocks are included
	assert.Contains(t, prompt, "All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.")
	assert.Contains(t, prompt, "User previously asked about database connections and prefers PostgreSQL")

	// Check that knowledge blocks are NOT included
	assert.NotContains(t, prompt, "All the knowledge provided to you is below. Use it as the context to answer the user's question.")

	// Check that important instructions are included
	assert.Contains(t, prompt, "You MUST use the provided tools to perform actions")
	assert.Contains(t, prompt, "If you need to provide a direct response, you must first use the appropriate tool to get the information")
}

func TestSkillContextRunnerPrompt_WithoutMemory(t *testing.T) {
	data := SkillContextRunnerPromptData{
		MainAgentSystemPrompt: "You are a helpful AI assistant",
		SkillSystemPrompt:     "You can perform basic calculations",
	}

	prompt, err := SkillContextRunnerPrompt(data)
	require.NoError(t, err)

	// Check that main system prompt is included
	assert.Contains(t, prompt, "You are a helpful AI assistant")

	// Check that skill system prompt is included
	assert.Contains(t, prompt, "You can perform basic calculations")

	// Check that neither memory nor knowledge blocks are included
	assert.NotContains(t, prompt, "All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.")
	assert.NotContains(t, prompt, "All the knowledge provided to you is below. Use it as the context to answer the user's question.")

	// Check that important instructions are still included
	assert.Contains(t, prompt, "You MUST use the provided tools to perform actions")
	assert.Contains(t, prompt, "Skip lengthy explanations, make decisions quickly")
	assert.Contains(t, prompt, "Help users understand your progress without lengthy details")
}

func TestSkillContextRunnerPrompt_EmptyStrings(t *testing.T) {
	data := SkillContextRunnerPromptData{
		MainAgentSystemPrompt: "",
		SkillSystemPrompt:     "",
		MemoryBlocks:          "",
	}

	prompt, err := SkillContextRunnerPrompt(data)
	require.NoError(t, err)

	// Check that the prompt is generated even with empty strings
	assert.NotEmpty(t, prompt)

	// Check that important instructions are still included
	assert.Contains(t, prompt, "IMPORTANT INSTRUCTIONS:")
	assert.Contains(t, prompt, "PERFORMANCE:")
	assert.Contains(t, prompt, "COMMUNICATION:")

	// Check that conditional sections are not included
	assert.NotContains(t, prompt, "All the memory learned from user's previous interactions are provided below")
	assert.NotContains(t, prompt, "All the knowledge provided to you is below")
}

func TestSkillContextRunnerPrompt_ComplexContent(t *testing.T) {
	data := SkillContextRunnerPromptData{
		MainAgentSystemPrompt: "You are an AI assistant specialized in Go programming and Kubernetes operations. You help developers with debugging, code review, and infrastructure management.",
		SkillSystemPrompt:     "You have access to: 1) Code analysis tools for Go, 2) Kubernetes cluster management, 3) Database query tools, 4) Log analysis capabilities",
		MemoryBlocks:          "User is working on a microservices architecture. Previous issues: connection timeouts in service mesh, database connection pooling problems, and log aggregation challenges.",
	}

	prompt, err := SkillContextRunnerPrompt(data)
	require.NoError(t, err)

	// Check that all complex content is properly included
	assert.Contains(t, prompt, "You are an AI assistant specialized in Go programming and Kubernetes operations")
	assert.Contains(t, prompt, "You have access to: 1) Code analysis tools for Go, 2) Kubernetes cluster management")
	assert.Contains(t, prompt, "User is working on a microservices architecture")

	// Check that the template structure is maintained
	assert.Contains(t, prompt, "IMPORTANT INSTRUCTIONS:")
	assert.Contains(t, prompt, "PERFORMANCE:")
	assert.Contains(t, prompt, "COMMUNICATION:")

	// Check that conditional sections are properly included
	assert.Contains(t, prompt, "All the memory learned from user's previous interactions are provided below")
}
