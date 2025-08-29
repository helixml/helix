package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_WithKnowledgeBlocks(t *testing.T) {
	prompt, err := NoSkillsPrompt(NoSkillsPromptData{
		MainAgentSystemPrompt: "You are a helpful assistant",
		KnowledgeBlocks:       "Knowledge",
	})
	assert.NoError(t, err)
	assert.Contains(t, prompt, "You are a helpful assistant")
	assert.Contains(t, prompt, "All the knowledge provided to you is below. Use it as the context to answer the user's question.")
	assert.Contains(t, prompt, "Knowledge")
}

func Test_WithMemoryBlocks(t *testing.T) {
	prompt, err := NoSkillsPrompt(NoSkillsPromptData{
		MainAgentSystemPrompt: "You are a helpful assistant",
		MemoryBlocks:          "Memory",
	})
	assert.NoError(t, err)
	assert.Contains(t, prompt, "You are a helpful assistant")
	assert.Contains(t, prompt, "All the memory learned from user's previous interactions are provided below. Use it as the context to answer the user's question.")
	assert.Contains(t, prompt, "Memory")
}

func Test_WithMemoryAndKnowledgeBlocks(t *testing.T) {
	prompt, err := NoSkillsPrompt(NoSkillsPromptData{
		MainAgentSystemPrompt: "You are a helpful assistant",
		MemoryBlocks:          "Memory",
		KnowledgeBlocks:       "Knowledge",
	})
	assert.NoError(t, err)
	assert.Contains(t, prompt, "You are a helpful assistant")
}
