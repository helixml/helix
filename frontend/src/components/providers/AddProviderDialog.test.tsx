import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AddProviderDialog from './AddProviderDialog'
import { PROVIDERS } from './types'

const createProviderEndpoint = vi.fn()
const updateProviderEndpoint = vi.fn()

vi.mock('../../services/providersService', () => ({
  useCreateProviderEndpoint: () => ({ mutateAsync: createProviderEndpoint, isPending: false }),
  useUpdateProviderEndpoint: () => ({ mutateAsync: updateProviderEndpoint, isPending: false }),
  useDeleteProviderEndpoint: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn() }),
}))

const anthropic = PROVIDERS.find((provider) => provider.id === 'user/anthropic')!

const baseUrlInput = () => screen.getByPlaceholderText('https://api.anthropic.com/v1')
const apiKeyInput = () => document.querySelector('input[type="password"]') as HTMLInputElement

describe('AddProviderDialog base URL', () => {
  beforeEach(() => {
    createProviderEndpoint.mockReset()
    updateProviderEndpoint.mockReset()
  })

  it('uses the default Anthropic URL when left blank', async () => {
    render(<AddProviderDialog open onClose={vi.fn()} orgId="" provider={anthropic} />)

    expect(baseUrlInput()).toHaveValue('')
    fireEvent.change(apiKeyInput(), { target: { value: 'sk-ant-test' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => expect(createProviderEndpoint).toHaveBeenCalledWith(expect.objectContaining({
      name: 'anthropic',
      base_url: 'https://api.anthropic.com/v1',
    })))
  })

  it('uses a custom Anthropic URL when provided', async () => {
    render(<AddProviderDialog open onClose={vi.fn()} orgId="" provider={anthropic} />)

    fireEvent.change(baseUrlInput(), { target: { value: 'https://gateway.example.com/anthropic/v1' } })
    fireEvent.change(apiKeyInput(), { target: { value: 'sk-ant-test' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => expect(createProviderEndpoint).toHaveBeenCalledWith(expect.objectContaining({
      name: 'anthropic',
      base_url: 'https://gateway.example.com/anthropic/v1',
    })))
  })

  it('shows an existing override and rejects invalid URLs', async () => {
    render(
      <AddProviderDialog
        open
        onClose={vi.fn()}
        orgId=""
        provider={anthropic}
        existingProvider={{ id: 'ep-1', name: 'anthropic', base_url: 'https://proxy.example.com/v1', api_key: 'sk-ant-existing' }}
      />,
    )

    expect(baseUrlInput()).toHaveValue('https://proxy.example.com/v1')
    fireEvent.change(baseUrlInput(), { target: { value: 'not a url' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update' }))

    expect(await screen.findByText('Please enter a valid URL')).toBeInTheDocument()
    expect(updateProviderEndpoint).not.toHaveBeenCalled()
  })

  it('is read-only for an env-configured system endpoint', () => {
    render(
      <AddProviderDialog
        open
        onClose={vi.fn()}
        orgId=""
        provider={anthropic}
        existingProvider={{ id: 'global/anthropic', name: 'anthropic', base_url: 'https://proxy.example.com/v1', owner: 'system' }}
      />,
    )

    expect(screen.getByText(/configured through environment variables/)).toHaveTextContent('ANTHROPIC_BASE_URL')
    expect(baseUrlInput()).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Update' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument()
  })
})
