import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HelixOrgSettings from './HelixOrgSettings'

const mocks = vi.hoisted(() => ({
  setSetting: vi.fn(),
  deleteSetting: vi.fn(),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../components/helix-org/GitHubAppPanel', () => ({ default: () => null }))
vi.mock('../components/helix-org/SlackIntegrationsPanel', () => ({ default: () => null }))
vi.mock('../services/helixOrgService', () => ({
  useHelixOrgSettings: () => ({
    data: {
      specs: [{ key: 'helix.url', type: 'string', configured: true, value: 'https://old.example' }],
    },
    isLoading: false,
  }),
  useSetHelixOrgSetting: () => ({ mutateAsync: mocks.setSetting, isPending: false }),
  useDeleteHelixOrgSetting: () => ({ mutateAsync: mocks.deleteSetting, isPending: false }),
}))

describe('HelixOrgSettings autosave', () => {
  beforeEach(() => {
    mocks.setSetting.mockReset()
    mocks.deleteSetting.mockReset()
  })

  it('saves edited and blank values on blur without a Save button', async () => {
    mocks.setSetting.mockResolvedValue(undefined)
    render(<HelixOrgSettings />)

    const input = screen.getByDisplayValue('https://old.example')
    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)

    await waitFor(() => expect(mocks.setSetting).toHaveBeenCalledWith({ key: 'helix.url', value: '' }))
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument()
  })

  it('shows a visible failure when autosave fails', async () => {
    mocks.setSetting.mockRejectedValue(new Error('network failed'))
    render(<HelixOrgSettings />)

    const input = screen.getByDisplayValue('https://old.example')
    fireEvent.change(input, { target: { value: 'https://new.example' } })
    fireEvent.blur(input)

    expect(await screen.findByText('Save failed')).toBeInTheDocument()
  })
})
