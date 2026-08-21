import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import EditProviderModelsDialog from './EditProviderModelsDialog'

const catalogueState = vi.hoisted(() => ({
  data: undefined as any,
  isLoading: false,
}))
const saved = vi.hoisted(() => ({ calls: [] as { id: string; models: string[] }[] }))
const refreshed = vi.hoisted(() => ({ calls: [] as string[] }))

vi.mock('../../services/providersService', () => ({
  useProviderAvailableModels: () => ({
    data: catalogueState.data,
    isLoading: catalogueState.isLoading,
    error: null,
    isRefetching: false,
  }),
  useUpdateProviderEndpointModels: () => ({
    mutateAsync: async (args: { id: string; models: string[] }) => {
      saved.calls.push(args)
      return {}
    },
    isPending: false,
  }),
  useRefreshProviderAvailableModels: () => ({
    mutate: (id: string) => {
      refreshed.calls.push(id)
    },
    isPending: false,
  }),
}))

const endpoint = { id: 'pe_1', name: 'OpenRouter', models: [] } as any

const catalogue = {
  enabled_models: [],
  models: [
    { id: 'anthropic/claude-opus-5', name: 'Anthropic: Claude Opus 5', context_length: 200000, supported_parameters: ['tools'] },
    { id: 'anthropic/claude-haiku-5', name: 'Anthropic: Claude Haiku 5', context_length: 200000, supported_parameters: ['tools'] },
    { id: 'openai/gpt-5.2', name: 'OpenAI: GPT-5.2', context_length: 400000, supported_parameters: ['tools'] },
    { id: 'meta/llama-4-base', name: 'Meta: Llama 4 Base', context_length: 128000, supported_parameters: [] },
  ],
}

function renderDialog(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

describe('EditProviderModelsDialog', () => {
  beforeEach(() => {
    catalogueState.data = catalogue
    catalogueState.isLoading = false
    saved.calls = []
    refreshed.calls = []
  })

  it('lists the provider catalogue instead of the endpoint\'s enabled subset', async () => {
    renderDialog(<EditProviderModelsDialog open endpoint={endpoint} onClose={vi.fn()} refreshData={vi.fn()} />)

    expect(await screen.findByText('Anthropic: Claude Opus 5')).toBeInTheDocument()
    expect(screen.getByText('OpenAI: GPT-5.2')).toBeInTheDocument()
    // Nothing enabled yet means the whole catalogue is available.
    expect(screen.getByText('All 4 models enabled')).toBeInTheDocument()
  })

  it('filters by search across id and display name', async () => {
    renderDialog(<EditProviderModelsDialog open endpoint={endpoint} onClose={vi.fn()} refreshData={vi.fn()} />)

    fireEvent.change(screen.getByPlaceholderText('Search models'), { target: { value: 'claude opus' } })

    await waitFor(() => expect(screen.queryByText('OpenAI: GPT-5.2')).not.toBeInTheDocument())
    expect(screen.getByText('Anthropic: Claude Opus 5')).toBeInTheDocument()
    expect(screen.queryByText('Anthropic: Claude Haiku 5')).not.toBeInTheDocument()
  })

  it('bulk-enables the filtered subset and saves it as the whitelist', async () => {
    const onClose = vi.fn()
    renderDialog(<EditProviderModelsDialog open endpoint={endpoint} onClose={onClose} refreshData={vi.fn()} />)

    fireEvent.change(screen.getByPlaceholderText('Search models'), { target: { value: 'anthropic' } })
    await waitFor(() => expect(screen.getByText('Enable 2 shown')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Enable 2 shown'))

    await waitFor(() => expect(screen.getByText('2 of 4 enabled')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(saved.calls).toHaveLength(1))
    expect(saved.calls[0]).toEqual({
      id: 'pe_1',
      models: ['anthropic/claude-opus-5', 'anthropic/claude-haiku-5'],
    })
    expect(onClose).toHaveBeenCalled()
  })

  it('keeps models the provider no longer advertises when they are already enabled', async () => {
    catalogueState.data = { ...catalogue, enabled_models: ['pinned/gone-from-upstream'] }
    renderDialog(<EditProviderModelsDialog open endpoint={endpoint} onClose={vi.fn()} refreshData={vi.fn()} />)

    await waitFor(() => expect(screen.getByText('1 of 4 enabled')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(saved.calls).toHaveLength(1))
    expect(saved.calls[0].models).toEqual(['pinned/gone-from-upstream'])
  })

  it('refetches the catalogue from upstream on refresh', async () => {
    renderDialog(<EditProviderModelsDialog open endpoint={endpoint} onClose={vi.fn()} refreshData={vi.fn()} />)

    fireEvent.click(screen.getByLabelText('Refresh model list'))
    expect(refreshed.calls).toEqual(['pe_1'])
  })
})
