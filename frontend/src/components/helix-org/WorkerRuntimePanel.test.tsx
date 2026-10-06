import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DefaultRuntimePanel from './WorkerRuntimePanel'

const mocks = vi.hoisted(() => ({
  set: vi.fn(),
  pending: false,
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
  useHelixOrgSettings: () => ({ data: mocks.settings, isLoading: false }),
  useSetHelixOrgSetting: () => ({ mutateAsync: mocks.set, isPending: mocks.pending }),
}))

vi.mock('../../services/codeAgentHarnessesService', () => ({
  useOrgCodeAgentHarnesses: () => ({ data: [], isLoading: false }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('./BotRuntimeForm', () => ({
  default: ({ value, onChange, showReasoningEffort, disabled }: any) => (
    <div>
      <span>{value.runtime}/{value.provider}/{value.model}/{value.reasoning_effort}</span>
      <span>{showReasoningEffort ? 'Reasoning enabled' : 'Reasoning hidden'}</span>
      <button disabled={disabled} onClick={() => onChange({ reasoning_effort: 'high' })}>Set high</button>
    </div>
  ),
}))

describe('Default Runtime panel', () => {
  beforeEach(() => {
    mocks.set.mockReset()
    mocks.set.mockResolvedValue(undefined)
    mocks.pending = false
  })

  it('saves the complete organization runtime only after Save', async () => {
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
    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })

  it('shows failure feedback when Save fails', async () => {
    let rejectUpdate!: (error: Error) => void
    mocks.set.mockImplementation(() => new Promise((_, reject) => {
      rejectUpdate = reject
    }))
    render(<DefaultRuntimePanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Set high' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save Default Runtime' }))
    expect(await screen.findByText('Saving...', { selector: '[role="status"]' })).toBeInTheDocument()
    act(() => rejectUpdate(new Error('network failed')))
    expect(await screen.findByText('Save failed')).toBeInTheDocument()
  })

  it('disables controls while a save is pending', () => {
    mocks.pending = true
    render(<DefaultRuntimePanel />)

    expect(screen.getByRole('button', { name: 'Set high' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  })
})
