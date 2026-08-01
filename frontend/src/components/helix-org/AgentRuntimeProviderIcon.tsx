import { FC } from 'react'

import AnthropicLogo from '../providers/logos/anthropic'
import OpenAILogo from '../providers/logos/openai'

const AgentRuntimeProviderIcon: FC<{ runtime: string }> = ({ runtime }) => {
  if (runtime === 'claude_code') {
    return <AnthropicLogo role="img" aria-label="Anthropic" width={12} height={12} />
  }
  if (runtime === 'codex_cli') {
    return <OpenAILogo role="img" aria-label="OpenAI" width={12} height={12} />
  }
  return null
}

export default AgentRuntimeProviderIcon
