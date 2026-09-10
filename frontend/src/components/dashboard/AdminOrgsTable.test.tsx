import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AdminOrgsTable from './AdminOrgsTable'

const mocks = vi.hoisted(() => ({
  queries: [] as Array<{ page?: number; per_page?: number; query?: string }>,
}))

vi.mock('../../services/dashboardService', () => ({
  useListAdminOrgs: (query: { page?: number; per_page?: number; query?: string }) => {
    mocks.queries.push(query)
    return {
      data: { organizations: [], totalCount: 73, page: query.page, pageSize: query.per_page, totalPages: 3 },
      isLoading: false,
      error: null,
    }
  },
  useAdminSetOrgPlan: () => ({ mutate: vi.fn(), isPending: false }),
}))

describe('AdminOrgsTable', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.queries = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('debounces search and resets pagination before refetching', () => {
    render(<AdminOrgsTable />)

    fireEvent.click(screen.getByRole('button', { name: 'Go to next page' }))
    expect(mocks.queries.at(-1)).toMatchObject({ page: 2, per_page: 25 })

    fireEvent.change(screen.getByLabelText('Search organizations'), { target: { value: 'acme' } })
    act(() => vi.advanceTimersByTime(300))

    expect(mocks.queries.at(-1)).toEqual({ page: 1, per_page: 25, query: 'acme' })
  })
})
