import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AdminOrgsTable from './AdminOrgsTable'

const mocks = vi.hoisted(() => ({
  queries: [] as Array<{ page?: number; per_page?: number; query?: string }>,
  organizations: [] as Array<Record<string, unknown>>,
}))

vi.mock('../../services/dashboardService', () => ({
  useListAdminOrgs: (query: { page?: number; per_page?: number; query?: string }) => {
    mocks.queries.push(query)
    return {
      data: { organizations: mocks.organizations, totalCount: 73, page: query.page, pageSize: query.per_page, totalPages: 3 },
      isLoading: false,
      error: null,
    }
  },
  useAdminSetOrgPlan: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useAdminActivateTrial: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAdminRevokeTrial: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAdminGrantCredits: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

describe('AdminOrgsTable', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.queries = []
    mocks.organizations = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('debounces search and resets pagination before refetching', () => {
    render(<AdminOrgsTable />)

    fireEvent.click(screen.getByRole('button', { name: 'Go to next page' }))
    expect(mocks.queries.at(-1)).toMatchObject({ page: 2, per_page: 25 })

    fireEvent.change(screen.getByLabelText('Search organizations or owner email'), { target: { value: 'acme' } })
    act(() => vi.advanceTimersByTime(300))

    expect(mocks.queries.at(-1)).toEqual({ page: 1, per_page: 25, query: 'acme' })
  })

  it('shows each member email with their organization role', () => {
    mocks.organizations = [{
      organization: { id: 'org-1', name: 'acme' },
      members: [
        { user_id: 'user-1', role: 'owner', user: { id: 'user-1', email: 'owner@example.com' } },
        { user_id: 'user-2', role: 'member', user: { id: 'user-2', email: 'member@example.com' } },
      ],
      projects: [],
    }]

    render(<AdminOrgsTable />)

    expect(screen.getByText('owner@example.com (Owner)')).toBeInTheDocument()
    expect(screen.getByText('member@example.com (Member)')).toBeInTheDocument()
  })
})
