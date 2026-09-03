import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import WorkerRuntimePanel from './WorkerRuntimePanel'

const mocks = vi.hoisted(() => ({
  setSetting: vi.fn(),
  settingsData: { specs: [] },
  pending: false,
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../../services/codeAgentHarnessesService', () => ({
  useOrgCodeAgentHarnesses: () => ({ data: [], isLoading: false }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useHelixOrgBase: () => ({ orgID: 'org-1' }),
  useHelixOrgSettings: () => ({ data: mocks.settingsData, isLoading: false }),
  useSetHelixOrgSetting: () => ({ mutateAsync: mocks.setSetting, isPending: mocks.pending }),
}))
vi.mock('./BotRuntimeForm', () => ({
  default: ({ onChange, disabled }: { onChange: (patch: { runtime: string }) => void; disabled: boolean }) => (
    <button disabled={disabled} onClick={() => { void onChange({ runtime: 'qwen_code' }) }}>Change runtime</button>
  ),
}))

describe('WorkerRuntimePanel save feedback', () => {
  beforeEach(() => {
    mocks.setSetting.mockReset()
    mocks.pending = false
  })

  it('shows saved feedback after an automatic update', async () => {
    mocks.setSetting.mockResolvedValue(undefined)
    render(<WorkerRuntimePanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Change runtime' }))

    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })

  it('shows failure feedback after an automatic update fails', async () => {
    let rejectUpdate!: (error: Error) => void
    mocks.setSetting.mockImplementation(() => new Promise((_, reject) => {
      rejectUpdate = reject
    }))
    render(<WorkerRuntimePanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Change runtime' }))

    expect(await screen.findByText('Saving...')).toBeInTheDocument()
    act(() => rejectUpdate(new Error('network failed')))
    expect(await screen.findByText('Save failed')).toBeInTheDocument()
  })

  it('disables controls while an update is pending', () => {
    mocks.pending = true
    render(<WorkerRuntimePanel />)

    expect(screen.getByRole('button', { name: 'Change runtime' })).toBeDisabled()
  })

})
