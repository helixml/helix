import React from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SystemSettingsTable from './SystemSettingsTable'

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  snackbarSuccess: vi.fn(),
  snackbarError: vi.fn(),
}))

vi.mock('../../services/systemSettingsService', () => ({
  useGetSystemSettings: () => ({
    data: {
      default_new_project_agent_provider: 'pe_current',
      default_new_project_agent_model: 'current-model',
      default_new_project_agent_reasoning_effort: 'medium',
      huggingface_token_source: 'none',
    },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  useUpdateSystemSettings: () => ({
    mutateAsync: mocks.mutateAsync,
    isPending: false,
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({
    success: mocks.snackbarSuccess,
    error: mocks.snackbarError,
    info: vi.fn(),
  }),
}))

vi.mock('../create/AdvancedModelPicker', () => ({
  default: ({ onSelectModel, hint }: { onSelectModel: (provider: string, model: string) => void; hint?: string }) => (
    <button type="button" aria-label={hint} onClick={() => onSelectModel('pe_selected', 'selected-model')}>
      Select model
    </button>
  ),
}))

describe('SystemSettingsTable default project agent settings', () => {
  beforeEach(() => {
    mocks.mutateAsync.mockReset()
    mocks.mutateAsync.mockResolvedValue({})
    mocks.snackbarSuccess.mockReset()
    mocks.snackbarError.mockReset()
  })

  it('updates the default provider, model, and reasoning effort', async () => {
    render(<SystemSettingsTable />)

    const row = screen.getByText('Default New Project Agent').closest('tr')
    expect(row).not.toBeNull()
    expect(within(row!).getByText('current-model')).toBeInTheDocument()
    expect(within(row!).queryByText(/pe_current/)).not.toBeInTheDocument()

    fireEvent.click(within(row!).getByRole('button', {
      name: 'Select the provider and model assigned to coding agents created with new projects.',
    }))
    await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledWith({
      default_new_project_agent_provider: 'pe_selected',
      default_new_project_agent_model: 'selected-model',
      default_new_project_agent_reasoning_effort: 'medium',
    }))

    fireEvent.mouseDown(within(row!).getByLabelText('Reasoning effort'))
    fireEvent.click(await screen.findByRole('option', { name: 'High' }))
    await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledWith({
      default_new_project_agent_reasoning_effort: 'high',
    }))
  })
})
