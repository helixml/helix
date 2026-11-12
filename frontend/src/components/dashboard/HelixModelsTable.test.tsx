import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HelixModelsTable from './HelixModelsTable'

const state = vi.hoisted(() => ({
  models: [] as Array<Record<string, unknown>>,
}))

vi.mock('../../services/helixModelsService', () => ({
  useListHelixModels: () => ({ data: state.models, isLoading: false, refetch: vi.fn() }),
  useUpdateHelixModel: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../../services/runnerProfilesService', () => ({
  useListInferenceModels: () => ({ data: undefined }),
}))

vi.mock('../../services/modelInfoService', () => ({
  useListModelInfos: () => ({ data: [], refetch: vi.fn() }),
}))

vi.mock('./EditHelixModel', () => ({ default: () => null }))
vi.mock('./DeleteHelixModelDialog', () => ({ default: () => null }))
vi.mock('./ModelPricingDialog', () => ({ default: () => null }))

const searchLabel = 'Search models by ID, name, or description'

describe('HelixModelsTable search', () => {
  beforeEach(() => {
    state.models = [
      { id: 'qwen3:8b', name: 'Qwen3 8B', description: 'Alibaba model', type: 'chat', runtime: 'ollama' },
      { id: 'gpt-oss:20b', name: 'GPT-OSS 20B', description: 'OpenAI model', type: 'chat', runtime: 'ollama' },
    ]
  })

  it('shows the empty state when there are no models at all', () => {
    state.models = []
    render(<HelixModelsTable />)

    expect(screen.getByText('No Helix models found.')).toBeInTheDocument()
    expect(screen.queryByLabelText(searchLabel)).not.toBeInTheDocument()
  })

  it('keeps the search box and header when a query matches nothing', () => {
    render(<HelixModelsTable />)
    fireEvent.change(screen.getByLabelText(searchLabel), { target: { value: 'zzznomatch' } })

    expect(screen.getByLabelText(searchLabel)).toHaveValue('zzznomatch')
    expect(screen.getByRole('columnheader', { name: 'ID' })).toBeInTheDocument()
    expect(screen.getByText('No models match "zzznomatch"')).toBeInTheDocument()
    expect(screen.queryByText('No Helix models found.')).not.toBeInTheDocument()
    expect(screen.queryByText('qwen3:8b')).not.toBeInTheDocument()
  })

  it('restores all rows when the search is cleared', () => {
    render(<HelixModelsTable />)
    fireEvent.change(screen.getByLabelText(searchLabel), { target: { value: 'zzznomatch' } })
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }))

    expect(screen.getByLabelText(searchLabel)).toHaveValue('')
    expect(screen.getByText('qwen3:8b')).toBeInTheDocument()
    expect(screen.getByText('gpt-oss:20b')).toBeInTheDocument()
    expect(screen.queryByText(/No models match/)).not.toBeInTheDocument()
  })
})
