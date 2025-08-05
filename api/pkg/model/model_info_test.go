package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetGeminiFlash(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "google",
		Model:    "models/gemini-2.0-flash-001",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.0 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000004", modelInfo.Pricing.Completion)
}

func Test_GetGeminiFlash_CustomUserProvider(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "pv_123",
		Model:    "google/gemini-2.5-flash",
		BaseURL:  "https://generativelanguage.googleapis.com/v1beta/openai",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.5 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000025", modelInfo.Pricing.Completion)
}

func Test_GetOpenAIo3Mini(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "openai",
		Model:    "o3-mini",
	})
	assert.NoError(t, err)

	assert.Equal(t, "OpenAI: o3 Mini", modelInfo.Name)
	assert.Equal(t, "0.0000044", modelInfo.Pricing.Completion)
}

func Test_GetOpenAIo3Mini_CustomUserProvider(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "id_123",
		BaseURL:  "https://api.openai.com/v1",
		Model:    "o3-mini",
	})
	assert.NoError(t, err)

	assert.Equal(t, "OpenAI: o3 Mini", modelInfo.Name)
	assert.Equal(t, "0.0000044", modelInfo.Pricing.Completion)
}
