import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import EmbedTaskPage from './EmbedTaskPage'

const mocks = vi.hoisted(() => ({
  query: { data: undefined as unknown, isLoading: false, error: null as unknown },
}))

vi.mock('react-router5', () => ({
  useRoute: () => ({ route: { params: { taskId: 'spt_1', controls: '0' } } }),
}))

vi.mock('../router', () => ({ initialQueryParams: {} }))

vi.mock('../services/specTaskService', () => ({
  useSpecTask: () => mocks.query,
}))

vi.mock('../components/tasks/SpecTaskDetailContent', () => ({
  default: () => <div>task workspace</div>,
}))

vi.mock('../components/session/AgentChat', () => ({
  default: () => <div>agent chat</div>,
}))

describe('EmbedTaskPage', () => {
  // A task that cannot be loaded must say so, not spin forever: a Helix
  // Recruiter customer's view spun indefinitely on a 403 (Helix #3455).
  it('shows the reason when the task cannot be loaded', () => {
    mocks.query = {
      data: undefined,
      isLoading: false,
      error: { response: { status: 403, data: 'Account is waiting for approval' } },
    }
    render(<EmbedTaskPage />)

    expect(screen.getByText(/isn't allowed to open its task/)).toBeTruthy()
    expect(screen.getByText('Account is waiting for approval')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Reload' })).toBeTruthy()
    expect(screen.queryByText('task workspace')).toBeNull()
  })

  it('shows the task once it loads', () => {
    mocks.query = { data: { id: 'spt_1' }, isLoading: false, error: null }
    render(<EmbedTaskPage />)

    expect(screen.getByText('task workspace')).toBeTruthy()
  })
})
