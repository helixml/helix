import { TypesCodeAgentRuntime } from '../api/api'

// Mirrors api/pkg/types/code_agent_models.go, as /api/v1/config serves it.
export const testCodeAgentModels = {
  [TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode]: [
    { id: 'claude-opus-5-5', name: 'Claude Opus 5.5 (1M context)', current: true },
    { id: 'claude-opus-5', name: 'Claude Opus 5 (1M context)', current: true },
    { id: 'claude-fable-5', name: 'Claude Fable 5 (1M context)', current: true },
    { id: 'claude-opus-4-8', name: 'Claude Opus 4.8 (1M context)', current: false },
    { id: 'sonnet', name: 'Claude Sonnet (latest)', current: false },
    { id: 'haiku', name: 'Claude Haiku (latest)', current: false },
  ],
  [TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI]: [
    { id: 'gpt-6-sol', name: 'GPT-6 Sol', current: true },
    { id: 'gpt-6-astra', name: 'GPT-6 Astra', current: true },
    { id: 'gpt-6-luna', name: 'GPT-6 Luna', current: true },
    { id: 'gpt-5.6-sol', name: 'GPT-5.6 Sol', current: true },
    { id: 'gpt-5.6-terra', name: 'GPT-5.6 Terra', current: true },
    { id: 'gpt-5.6-luna', name: 'GPT-5.6 Luna', current: true },
    { id: 'gpt-5.5', name: 'GPT-5.5', current: false },
    { id: 'gpt-5.4', name: 'GPT-5.4', current: false },
    { id: 'gpt-5.4-mini', name: 'GPT-5.4 Mini', current: false },
    { id: 'gpt-5.3-codex-spark', name: 'GPT-5.3 Codex Spark', current: false },
  ],
}
