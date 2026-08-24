import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TypesCodeAgentCredentialType, TypesCodeAgentRuntime, TypesSandboxRuntime } from '../../api/api'
import { AppsContext, IAppsContext } from '../../contexts/apps'
import type { IApp } from '../../types'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'
import { DEFAULT_SANDBOX_PRESET } from '../../constants/sandboxPresets'

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

  it('uses the task configuration for agent, compute, and environment details', async () => {
    render(
      <AppsContext.Provider value={appsContext([])}>
        <ProjectChatItemTooltip
          item={{
            id: 'task-codex',
            kind: 'spec-task',
            title: 'Fix CI',
            task: {
              id: 'task-codex',
              code_agent_config: {
                runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
                credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
                model: 'gpt-5.6-sol',
              },
              sandbox_resource_overrides: { vcpus: 8, memory_mb: 16384 },
              sandbox_runtime: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
            },
          }}
        >
          <button>Spec task row</button>
        </ProjectChatItemTooltip>
      </AppsContext.Provider>,
    )

    fireEvent.mouseOver(screen.getByRole('button', { name: 'Spec task row' }))

    expect(await screen.findByText('Codex')).toBeInTheDocument()
    expect(screen.getByText('gpt-5.6-sol')).toBeInTheDocument()
    expect(screen.getByText('8 vCPU · 16 GB RAM')).toBeInTheDocument()
    expect(screen.getByText('Headless')).toBeInTheDocument()
  })

  it('shows legacy spec tasks with the default compute and desktop environment', async () => {
    render(
      <ProjectChatItemTooltip
        item={{
          id: 'task-defaults',
          kind: 'spec-task',
          title: 'Legacy task',
          task: { id: 'task-defaults' },
        }}
      >
        <button>Legacy task row</button>
      </ProjectChatItemTooltip>,
    )

    fireEvent.mouseOver(screen.getByRole('button', { name: 'Legacy task row' }))

    expect(await screen.findByText(
      `${DEFAULT_SANDBOX_PRESET.vcpus} vCPU · ${DEFAULT_SANDBOX_PRESET.memory_mb / 1024} GB RAM`,
    )).toBeInTheDocument()
    expect(screen.getByText('Full Desktop')).toBeInTheDocument()
  })
})
