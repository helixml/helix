import { render, screen } from '@testing-library/react'
import { ReactFlowProvider } from '@xyflow/react'
import { describe, expect, it, vi } from 'vitest'

import { AssetNode, buildGraph } from './HelixOrgChart'

const handlers = {
  onSelectBot: vi.fn(),
  onOpenBotDetails: vi.fn(),
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

const asset = {
  id: 'a-server',
  name: 'ubuntu-1',
  kind: 'server',
  agent_ids: ['chief-of-staff'],
  server: { address: '10.0.0.8', port: 22, user: 'ubuntu', auth_type: 'ssh_key' },
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

    expect(graph.nodes.find((node) => node.id === 'asset:a-server')?.position).toEqual({ x: 123, y: 456 })
    expect(graph.edges).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'asset-link:a-server->chief-of-staff',
        source: 'asset:a-server',
        target: 'bot:chief-of-staff',
        markerEnd: expect.objectContaining({ color: 'rgba(25,118,210,0.6)' }),
      }),
    ]))
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
    expect(screen.getByRole('status')).toHaveAccessibleName('Network or SSH unavailable')
    expect(screen.getByText('1 allowed agent')).toBeInTheDocument()
  })
})
