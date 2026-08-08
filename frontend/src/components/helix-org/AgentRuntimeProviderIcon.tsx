import { FC } from 'react'
import AgentHarness from '../agent/AgentHarness'

const AgentRuntimeProviderIcon: FC<{ runtime: string; size?: number }> = ({ runtime, size = 16 }) => {
  return <AgentHarness runtime={runtime} variant="short" size={size} />
}

export default AgentRuntimeProviderIcon
