import { useCallback } from 'react'
import { TypesCodeAgentExecutionConfig, TypesCodeAgentOverrides } from '../api/api'
import useSnackbar from './useSnackbar'

// Both execution-config endpoints take the same coding-identity change and
// report the same way, so the surfaces that mount SpecTaskExecutionControls
// share one handler rather than each re-deriving the request and the wording.
interface CodeAgentConfigRequest {
  agent_id?: string
  code_agent_config?: TypesCodeAgentExecutionConfig
  code_agent_overrides?: TypesCodeAgentOverrides
}

interface CodeAgentConfigResult {
  agent_thread_restarted?: boolean
}

/**
 * Builds the `onAgentModelChange` handler for SpecTaskExecutionControls.
 *
 * Pass the `mutateAsync` of whichever execution-config mutation owns the
 * surface — useUpdateSpecTaskExecutionConfig on the task page,
 * useUpdateSessionExecutionConfig in a chat session.
 */
export default function useCodeAgentConfigChange(
  mutateAsync: (request: CodeAgentConfigRequest) => Promise<CodeAgentConfigResult | undefined>,
) {
  const snackbar = useSnackbar()

  return useCallback(async (
    agentId: string,
    codeAgentOverrides: TypesCodeAgentOverrides,
    codeAgentConfig?: TypesCodeAgentExecutionConfig,
  ) => {
    const result = await mutateAsync(codeAgentConfig
      ? { code_agent_config: codeAgentConfig }
      : { agent_id: agentId, code_agent_overrides: codeAgentOverrides })
    snackbar.success(
      result?.agent_thread_restarted
        ? 'Coding configuration updated — a new agent thread is starting'
        : 'Coding configuration updated',
    )
  }, [mutateAsync, snackbar])
}
