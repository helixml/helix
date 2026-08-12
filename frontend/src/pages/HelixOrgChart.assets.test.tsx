import { fireEvent, render, screen } from '@testing-library/react'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import { ReactFlowProvider } from '@xyflow/react'
import { describe, expect, it, vi } from 'vitest'

import { AssetAuthType, AssetKind } from '../api/api'
import type { AssetDTO } from '../services/helixOrgService'
import { AssetNode, assetLinkFromConnection, BotNode, buildGraph, PeoplePanel } from './HelixOrgChart'

const handlers = {
  onSelectBot: vi.fn(),
  onOpenBotDetails: vi.fn(),
  onViewProject: vi.fn(),
  onNewBot: vi.fn(),
  onDeleteBot: vi.fn(),
  onStartBot: vi.fn(),
  onStopBot: vi.fn(),
  onRestartBot: vi.fn(),
  onSelectTopic: vi.fn(),
  onDeleteTopic: vi.fn(),
  onSelectProcessor: vi.fn(),
  onDeleteProcessor: vi.fn(),
  onSelectAsset: vi.fn(),
  onDeleteAsset: vi.fn(),
}

const asset: AssetDTO = {
  id: 'a-server',
  name: 'ubuntu-1',
  kind: AssetKind.KindServer,
  agent_ids: ['chief-of-staff'],
  server: { address: '10.0.0.8', port: 22, user: 'ubuntu', auth_type: AssetAuthType.AuthSSHKey },
}

describe('HelixOrgChart server assets', () => {
  it('builds a positioned asset node and a visible agent link', () => {
    const graph = buildGraph(
      [{
        id: 'chief-of-staff', name: 'Chief of Staff', parentIds: [], agentStatus: 'running',
        agentRuntime: 'zed_external', agentModel: 'test', taskStats: { backlog: 0, inProgress: 0, done: 0 },
      }],
      handlers,
      true,
      [],
      {},
      [],
      [asset],
      {},
      new Set(),
      { 'asset:a-server': { x: 123, y: 456 } },
    )

    const assetNode = graph.nodes.find((node) => node.id === 'asset:a-server')
    const agentNode = graph.nodes.find((node) => node.id === 'bot:chief-of-staff')
    expect(assetNode?.position).toEqual({ x: 123, y: 456 })
    expect(assetNode?.connectable).toBe(true)
    expect(assetNode).toMatchObject({ initialWidth: 220, initialHeight: 100 })
    expect(agentNode).toMatchObject({ initialWidth: 220, initialHeight: 96 })
    expect(graph.edges).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'asset-link:a-server->chief-of-staff',
        source: 'asset:a-server',
        target: 'bot:chief-of-staff',
        markerEnd: expect.objectContaining({ color: 'rgba(25,118,210,0.6)' }),
      }),
    ]))
  })

  it('routes connections dragged from an asset into an agent', () => {
    expect(assetLinkFromConnection('asset:a-server', 'bot:chief-of-staff')).toEqual({
      assetId: 'a-server',
      agentId: 'chief-of-staff',
    })
    expect(assetLinkFromConnection('bot:chief-of-staff', 'asset:a-server')).toBeNull()
  })

  it('renders one combined network and SSH health indicator', () => {
    render(
      <ReactFlowProvider>
        <AssetNode {...({
          id: 'asset:a-server',
          data: {
            asset,
            health: { tcp_reachable: true, ssh_reachable: false },
            onSelectAsset: handlers.onSelectAsset,
            onDeleteAsset: handlers.onDeleteAsset,
          },
        } as any)} />
      </ReactFlowProvider>,
    )

    expect(screen.getAllByRole('status')).toHaveLength(1)
    expect(screen.getAllByLabelText(/Connect asset to an agent from the/)).toHaveLength(4)
    for (const side of ['left', 'right', 'top', 'bottom']) {
      expect(screen.getByLabelText(`Connect asset to an agent from the ${side}`)).toHaveStyle({ width: '10px', height: '10px' })
    }
    expect(screen.getByRole('status')).toHaveAccessibleName('Network or SSH unavailable')
    expect(screen.getByText('1 allowed agent')).toBeInTheDocument()
  })
})

describe('HelixOrgChart agent actions', () => {
  it('opens the agent project from the node menu', () => {
    render(
      <ReactFlowProvider>
        <BotNode {...({
          id: 'bot:chief-of-staff',
          data: {
            botId: 'chief-of-staff',
            botName: 'Chief of Staff',
            agentStatus: 'running',
            agentRuntime: 'zed_external',
            agentModel: 'test',
            projectId: 'project-1',
            taskStats: { backlog: 0, inProgress: 0, done: 0 },
            selected: false,
            ...handlers,
          },
        } as any)} />
      </ReactFlowProvider>,
    )

    fireEvent.contextMenu(screen.getByText('Chief of Staff'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Settings' }))
    expect(handlers.onOpenBotDetails).toHaveBeenCalledWith('chief-of-staff')

    fireEvent.contextMenu(screen.getByText('Chief of Staff'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'View project' }))

    expect(handlers.onViewProject).toHaveBeenCalledWith('project-1')
  })

  it('renders quick settings and project actions on the node', () => {
    render(
      <ReactFlowProvider>
        <BotNode {...({
          id: 'bot:chief-of-staff',
          data: {
            botId: 'chief-of-staff',
            botName: 'Chief of Staff',
            agentStatus: 'running',
            agentRuntime: 'zed_external',
            agentModel: 'test',
            projectId: 'project-1',
            taskStats: { backlog: 0, inProgress: 0, done: 0 },
            selected: true,
            ...handlers,
          },
        } as any)} />
      </ReactFlowProvider>,
    )

    expect(screen.getByRole('group', { name: 'Chief of Staff navigation' })).toBeInTheDocument()
    expect(getComputedStyle(screen.getByRole('group', { name: 'Chief of Staff navigation' })).opacity).toBe('1')
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('button', { name: 'Project' }))

    expect(handlers.onOpenBotDetails).toHaveBeenCalledWith('chief-of-staff')
    expect(handlers.onViewProject).toHaveBeenCalledWith('project-1')
  })
})

describe('HelixOrgChart processors and people panel', () => {
  it('uses nearest-side edges for processor inputs, chains, and outputs', () => {
    const graph = buildGraph(
      [{
        id: 'worker', name: 'Worker', parentIds: [], agentStatus: 'stopped',
        agentRuntime: 'zed_external', agentModel: 'test', taskStats: { backlog: 0, inProgress: 0, done: 0 },
      }],
      handlers,
      true,
      [
        { id: 'topic-input', name: 'Input', kind: 'stream', created_by: 'worker', subscribers: [] },
        { id: 's-first', name: 'First output', kind: 'stream', subscribers: ['worker'] },
        { id: 's-second', name: 'Second output', kind: 'stream', subscribers: [] },
      ],
      {},
      [
        { id: 'p-first', name: 'First', kind: 'template', inputTopicId: 'topic-input', outputs: [{ topicId: 's-first', label: 'default', match: '', owned: true }] },
        { id: 'p-second', name: 'Second', kind: 'template', inputTopicId: 's-first', outputs: [{ topicId: 's-second', label: 'default', match: '', owned: true }] },
      ],
      [],
      {},
      new Set(['topic-input']),
    )

    expect(graph.edges).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'procin:topic-input->p-first', type: 'closest' }),
      expect.objectContaining({ id: 'procchain:p-first:s-first->p-second', type: 'closest' }),
      expect.objectContaining({ id: 'procout:p-first:s-first->worker', type: 'closest' }),
    ]))
  })

  it('collapses and expands the people panel', () => {
    render(
      <ThemeProvider theme={createTheme()}>
        <PeoplePanel
          people={[{ id: 'alice', name: 'Alice', kind: 'human', identity: { email: 'alice@example.com' } } as any]}
          onSelect={handlers.onSelectBot}
        />
      </ThemeProvider>,
    )

    expect(screen.getByText('Alice')).toBeInTheDocument()
    fireEvent.click(screen.getByText('People'))
    expect(screen.queryByText('Alice')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('People'))
    expect(screen.getByText('Alice')).toBeInTheDocument()
  })
})
