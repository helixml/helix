// Recommended models for coding agent creation, ordered by preference.
// Used by NewSpecTaskForm, CreateProjectDialog, AgentSelectionModal,
// Onboarding, and ProjectSettings.
export const RECOMMENDED_CODING_MODELS = [
  // Anthropic
  "claude-opus-4-6",
  "claude-sonnet-4-6",
  "claude-haiku-4-6",
  "claude-opus-4-5-20251101",
  "claude-sonnet-4-5-20250929",
  "claude-haiku-4-5-20251001",
  // OpenAI
  "openai/gpt-5.1-codex",
  "openai/gpt-oss-120b",
  // Google Gemini
  "gemini-2.5-pro",
  "gemini-2.5-flash",
  // Zhipu GLM
  "glm-4.6",
  // Qwen (Coder + Large)
  "Qwen/Qwen3-Coder-480B-A35B-Instruct",
  "Qwen/Qwen3-Coder-30B-A3B-Instruct",
  "Qwen/Qwen3-235B-A22B-fp8-tput",
];