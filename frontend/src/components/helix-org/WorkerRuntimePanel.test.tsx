import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DefaultRuntimePanel from './WorkerRuntimePanel'

const mocks = vi.hoisted(() => ({
  set: vi.fn(),
  settings: {
    specs: [{
      key: 'agent.default',
      value: JSON.stringify({
        code_agent_runtime: 'zed_agent',
        code_agent_credential_type: 'api_key',
        provider: 'pe_helix',
        model: 'helix-model',
        reasoning_effort: 'medium',
      }),
    }],
  },
}))

vi.mock('../../services/helixOrgService', () => ({
  useHelixOrgBase: () => ({ orgID: 'org-1' }),
  useHelixOrgSettings: () => ({
    data: mocks.settings,
    isLoading: false,
  }),
  useSetHelixOrgSetting: () => ({ mutateAsync: mocks.set }),
}))

vi.mock('../../services/codeAgentHarnessesService', () => ({
  useOrgCodeAgentHarnesses: () => ({ data: [], isLoading: false }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('./BotRuntimeForm', () => ({
  default: ({ value, onChange, showReasoningEffort }: any) => (
    <div>
      <span>{value.runtime}/{value.provider}/{value.model}/{value.reasoning_effort}</span>
      <span>{showReasoningEffort ? 'Reasoning enabled' : 'Reasoning hidden'}</span>
      <button onClick={() => onChange({ reasoning_effort: 'high' })}>Set high</button>
    </div>
  ),
}))

describe('Default Runtime panel', () => {
  beforeEach(() => {
    mocks.set.mockReset()
    mocks.set.mockResolvedValue(undefined)
  })

  it('shows and saves the complete organization runtime', async () => {
    render(<DefaultRuntimePanel />)

    expect(screen.getByText('zed_agent/pe_helix/helix-model/medium')).toBeInTheDocument()
    expect(screen.getByText('Reasoning enabled')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Set high' }))
    expect(mocks.set).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save Default Runtime' }))

    await waitFor(() => expect(mocks.set).toHaveBeenCalledWith({
      key: 'agent.default',
      value: JSON.stringify({
        code_agent_runtime: 'zed_agent',
        code_agent_credential_type: 'api_key',
        provider: 'pe_helix',
        model: 'helix-model',
        reasoning_effort: 'high',
      }),
    }))
  })
})
