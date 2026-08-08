import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TypesCodeAgentRuntime } from '../../api/api'
import { AppsContext, IAppsContext } from '../../contexts/apps'
import type { IApp } from '../../types'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'

const appsContext = (apps: IApp[]): IAppsContext => ({
  apps,
  app: undefined,
  loadApps: async () => {},
  loadApp: async () => {},
  setApp: () => {},
  createAgent: async () => undefined,
  updateApp: async () => undefined,
  deleteApp: async () => undefined,
})

describe('ProjectChatItemTooltip', () => {
  it('shows repository, branch, harness, and model to the right on hover', async () => {
    render(
      <ProjectChatItemTooltip
        item={{
          id: 'task-one',
          kind: 'spec-task',
          title: 'Add chat details',
          session: {
            session_id: 'session-one',
            model_name: 'claude-opus-4-6',
            metadata: { code_agent_runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode },
          },
        }}
        repository="helix"
        branch="feature/chat-details"
      >
        <button>Chat row</button>
      </ProjectChatItemTooltip>,
    )

    fireEvent.mouseOver(screen.getByRole('button', { name: 'Chat row' }))

    expect(await screen.findByText('Add chat details')).toBeInTheDocument()
    expect(screen.getByText('helix')).toBeInTheDocument()
    expect(screen.getByText('feature/chat-details')).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByText('claude-opus-4-6')).toBeInTheDocument()
  })

  it('uses the agent configured on a spec task when no session summary is available', async () => {
    const agent = {
      id: 'app-codex',
      config: { helix: { assistants: [{ code_agent_runtime: 'codex_cli', model: 'gpt-5.6-sol' }] } },
    } as IApp
    render(
      <AppsContext.Provider value={appsContext([agent])}>
        <ProjectChatItemTooltip
          item={{
            id: 'task-codex',
            kind: 'spec-task',
            title: 'Fix CI',
            task: { id: 'task-codex', helix_app_id: agent.id },
          }}
        >
          <button>Spec task row</button>
        </ProjectChatItemTooltip>
      </AppsContext.Provider>,
    )

    fireEvent.mouseOver(screen.getByRole('button', { name: 'Spec task row' }))

    expect(await screen.findByText('Codex')).toBeInTheDocument()
    expect(screen.getByText('gpt-5.6-sol')).toBeInTheDocument()
  })
})
