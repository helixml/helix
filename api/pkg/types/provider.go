package types

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderTogetherAI Provider = "togetherai"
	ProviderHelix      Provider = "helix"
	ProviderVLLM       Provider = "vllm"
)
