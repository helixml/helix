import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import AgentDropdown from './AgentDropdown'
import { AGENT_KIND_CODING, AGENT_KIND_HELIX, AGENT_KIND_ORG, IApp } from '../../types'

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({ orgNavigate: vi.fn() }),
}))

const makeApp = (id: string, name: string, runtime: string, agentKind: string): IApp =>
  ({
    id,
    agent_kind: agentKind,
    config: {
      helix: {
        name,
        assistants: [{ code_agent_runtime: runtime }],
      },
    },
  } as unknown as IApp)

const CODING_AGENTS = [
  makeApp('app_zed', 'zed-codex', 'zed_agent', AGENT_KIND_CODING),
  makeApp('app_codex', 'Codex', 'codex_cli', AGENT_KIND_CODING),
  makeApp('app_claude', 'my-helper', 'claude_code', AGENT_KIND_CODING),
]

describe('AgentDropdown', () => {
  it('shows the harness mark for the selected agent in the closed state', () => {
    render(<AgentDropdown value="app_zed" onChange={vi.fn()} agents={CODING_AGENTS} />)

    // The collapsed control used to render a bare name with no clue which
    // harness the agent runs on.
    expect(screen.getByText('zed-codex')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Zed Agent' })).toBeInTheDocument()
  })

  it('renders each option with its own harness mark, not one generic icon', () => {
    render(<AgentDropdown value="" onChange={vi.fn()} agents={CODING_AGENTS} />)

    fireEvent.mouseDown(screen.getByRole('combobox'))
    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(3)

    expect(within(options[0]).getByRole('img', { name: 'Zed Agent' })).toBeInTheDocument()
    expect(within(options[1]).getByRole('img', { name: 'Codex' })).toBeInTheDocument()
    expect(within(options[2]).getByRole('img', { name: 'Claude Code' })).toBeInTheDocument()
  })

  it('labels the harness when the agent name does not already say it', () => {
    render(<AgentDropdown value="" onChange={vi.fn()} agents={CODING_AGENTS} />)

    fireEvent.mouseDown(screen.getByRole('combobox'))
    const options = screen.getAllByRole('option')

    expect(within(options[2]).getByText('Claude Code')).toBeInTheDocument()
    // "Codex" is already the agent's name — don't print it twice.
    expect(within(options[1]).getAllByText('Codex')).toHaveLength(1)
  })

  it('filters out org and helix agents so unselectable ones are never offered', () => {
    render(
      <AgentDropdown
        value=""
        onChange={vi.fn()}
        agents={[
          ...CODING_AGENTS,
          makeApp('app_bot', 'chief-of-staff', 'zed_agent', AGENT_KIND_ORG),
          makeApp('app_chat', 'chat-agent', 'zed_agent', AGENT_KIND_HELIX),
        ]}
      />,
    )

    fireEvent.mouseDown(screen.getByRole('combobox'))
    expect(screen.getAllByRole('option')).toHaveLength(3)
    expect(screen.queryByText('chief-of-staff')).not.toBeInTheDocument()
    expect(screen.queryByText('chat-agent')).not.toBeInTheDocument()
  })

  it('reports the picked agent id', () => {
    const onChange = vi.fn()
    render(<AgentDropdown value="" onChange={onChange} agents={CODING_AGENTS} />)

    fireEvent.mouseDown(screen.getByRole('combobox'))
    fireEvent.click(screen.getByText('my-helper'))

    expect(onChange).toHaveBeenCalledWith('app_claude')
  })

  it('shows an empty state when no coding agent exists', () => {
    render(
      <AgentDropdown
        value=""
        onChange={vi.fn()}
        agents={[makeApp('app_bot', 'chief-of-staff', 'zed_agent', AGENT_KIND_ORG)]}
      />,
    )

    fireEvent.mouseDown(screen.getByRole('combobox'))
    expect(screen.getByText('No agents available')).toBeInTheDocument()
  })
})
