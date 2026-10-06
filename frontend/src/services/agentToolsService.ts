import { useQuery } from '@tanstack/react-query'

import useApi from '../hooks/useApi'
import { TypesAgentToolInfo } from '../api/api'

export const AGENT_TOOL_CATALOGUE_KEY = ['agent-tools', 'catalogue']

// useAgentToolCatalogue lists the Helix MCP tools that can be granted to spec
// tasks. The catalogue is static per deployment, so it is cached aggressively.
export function useAgentToolCatalogue() {
  const api = useApi()
  return useQuery<TypesAgentToolInfo[]>({
    queryKey: AGENT_TOOL_CATALOGUE_KEY,
    queryFn: async () => {
      const res = await api.getApiClient().v1AgentToolsList()
      return (res.data ?? []) as TypesAgentToolInfo[]
    },
    staleTime: 60 * 60 * 1000,
  })
}
