import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SystemSettingsTable from './SystemSettingsTable'

const state = vi.hoisted(() => ({
  desktopLimit: 0,
  updates: [] as Array<Record<string, unknown>>,
}))

vi.mock('../../services/systemSettingsService', () => ({
  useGetSystemSettings: () => ({
    data: { max_concurrent_desktop_sandboxes: state.desktopLimit },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  useUpdateSystemSettings: () => ({
    mutateAsync: async (update: Record<string, unknown>) => state.updates.push(update),
    isPending: false,
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

vi.mock('../create/AdvancedModelPicker', () => ({ default: () => null }))

describe('SystemSettingsTable desktop sandbox limit', () => {
  beforeEach(() => {
    state.desktopLimit = 0
    state.updates = []
  })

  it('shows plan defaults and leaves the override empty', () => {
    render(<SystemSettingsTable />)
    const row = screen.getByText('Desktop Sandbox Limit').closest('tr')!

    expect(within(row).getAllByText('Plan defaults')).toHaveLength(2)
    fireEvent.click(within(row).getByRole('button', { name: 'Edit' }))

    expect(within(row).getByRole('textbox')).toHaveValue('')
    expect(within(row).getByText('Positive limit overrides plan defaults')).toBeInTheDocument()
  })

  it('edits and saves an explicit override', async () => {
    state.desktopLimit = 17
    render(<SystemSettingsTable />)
    const row = screen.getByText('Desktop Sandbox Limit').closest('tr')!

    expect(within(row).getByText('17 concurrent')).toBeInTheDocument()
    fireEvent.click(within(row).getByRole('button', { name: 'Edit' }))
    const input = within(row).getByRole('textbox')
    expect(input).toHaveValue('17')
    fireEvent.change(input, { target: { value: '23' } })
    fireEvent.click(within(row).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(state.updates).toEqual([{ max_concurrent_desktop_sandboxes: 23 }]))
  })
})
