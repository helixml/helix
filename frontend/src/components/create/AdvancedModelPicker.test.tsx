import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdvancedModelPicker from './AdvancedModelPicker'

const providerState = vi.hoisted(() => ({ providers: [] as any[] }))

vi.mock('../../services/providersService', () => ({
  useListProviders: () => ({ data: providerState.providers, isLoading: false }),
}))
vi.mock('../../services/userService', () => ({
  useGetUserTokenUsage: () => ({ data: undefined, isLoading: false }),
}))
vi.mock('../../services/orgService', () => ({
  useGetOrgByName: () => ({ data: undefined, isLoading: false }),
}))
vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ params: {} }),
}))

describe('AdvancedModelPicker', () => {
  beforeEach(() => {
    providerState.providers = [
      {
        id: 'openai-provider',
        name: 'OpenAI',
        base_url: 'https://api.openai.com/v1',
        available_models: [
          { id: 'gpt-5.6-sol', enabled: true, type: 'chat' },
          { id: 'gpt-4o', enabled: true, type: 'chat' },
        ],
      },
      {
        id: 'anthropic-provider',
        name: 'Anthropic',
        base_url: 'https://api.anthropic.com/v1',
        available_models: [
          { id: 'claude-opus-5', enabled: true, type: 'chat' },
          { id: 'claude-opus-4-1', enabled: true, type: 'chat' },
        ],
      },
      {
        id: 'custom-provider',
        name: 'Custom',
        base_url: 'http://models.local/v1',
        available_models: [
          { id: 'gpt-4-custom', enabled: true, type: 'chat' },
        ],
      },
    ]
  })

  it('collapses native legacy models while keeping custom models and search matches visible', () => {
    render(
      <AdvancedModelPicker
        currentType="chat"
        selectedModelId="gpt-5.6-sol"
        selectedProvider="openai-provider"
        onSelectModel={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /gpt-5.6-sol/i }))
    const dialog = within(screen.getByRole('dialog'))

    expect(dialog.getByText('gpt-5.6-sol')).toBeInTheDocument()
    expect(dialog.getByText('claude-opus-5')).toBeInTheDocument()
    expect(dialog.getByText('gpt-4-custom')).toBeInTheDocument()
    expect(dialog.queryByText('gpt-4o')).not.toBeInTheDocument()
    expect(dialog.queryByText('claude-opus-4-1')).not.toBeInTheDocument()
    const legacyButton = dialog.getByRole('button', { name: 'Show Legacy models' })
    expect(legacyButton).toHaveTextContent('Legacy models (2)')
    fireEvent.click(legacyButton)
    expect(dialog.getByText('gpt-4o')).toBeInTheDocument()
    expect(dialog.getByText('claude-opus-4-1')).toBeInTheDocument()
    fireEvent.click(dialog.getByRole('button', { name: 'Hide Legacy models' }))

    fireEvent.change(dialog.getByPlaceholderText('Search models...'), { target: { value: 'gpt-4o' } })
    expect(dialog.getByText('gpt-4o')).toBeInTheDocument()
  })

  it('keeps a selected legacy model outside the collapsed section', () => {
    render(
      <AdvancedModelPicker
        currentType="chat"
        selectedModelId="gpt-4o"
        selectedProvider="openai-provider"
        onSelectModel={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /gpt-4o/i }))
    const dialog = within(screen.getByRole('dialog'))

    expect(dialog.getByText('gpt-4o')).toBeInTheDocument()
    expect(dialog.getByRole('button', { name: 'Show Legacy models' })).toHaveTextContent('Legacy models (1)')
  })
})
