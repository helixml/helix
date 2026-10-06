import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { DevContainerCard } from './Runners'

vi.mock('../../contexts/account', () => ({
  useAccount: () => ({ orgNavigate: vi.fn() }),
}))

const container = {
  session_id: 'ses_ordinary',
  container_id: 'ctr_ordinary',
  container_name: 'ordinary',
  status: 'running',
  container_type: 'ubuntu',
  sandbox_id: 'runner_1',
}

describe('DevContainerCard', () => {
  it('shows the backend title without a stop control for web services', () => {
    render(
      <DevContainerCard
        container={{ ...container, session_id: 'sbx_web', purpose: 'web-service', session_name: 'Web service: FindAI' }}
        onStop={vi.fn()}
        isStopping={false}
      />,
    )

    expect(screen.getByText('Web service: FindAI')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Stop container' })).not.toBeInTheDocument()
  })

  it('keeps the stop control for ordinary sessions', () => {
    render(
      <DevContainerCard
        container={{ ...container, session_name: 'Coding session' }}
        onStop={vi.fn()}
        isStopping={false}
      />,
    )

    expect(screen.getByText('Coding session')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Stop container' })).toBeInTheDocument()
  })
})
