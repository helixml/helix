import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrgAgentSessionWorkspace, { botSandboxIndicatorState } from './OrgAgentSessionWorkspace'

const mocks = vi.hoisted(() => ({
  isBigScreen: true,
  resizedTo: vi.fn(),
}))

vi.mock('../../hooks/useIsBigScreen', () => ({ default: () => mocks.isBigScreen }))
vi.mock('../../hooks/useLightTheme', () => ({ default: () => ({ isLight: true }) }))
vi.mock('../../hooks/useIsPhone', () => ({ default: () => false }))
vi.mock('../../hooks/useElementWidth', () => ({ useElementWidth: () => [{ current: null }, 900] }))
vi.mock('../external-agent/ExternalAgentDesktopViewer', () => ({
  default: ({ sessionId }: { sessionId: string }) => <div>Desktop for {sessionId}</div>,
}))
vi.mock('../tasks/DiffViewer', () => ({
  default: ({ sessionId, primarySurface }: { sessionId: string; primarySurface: string }) => (
    <div>{primarySurface} for {sessionId}</div>
  ),
}))
vi.mock('../tasks/SandboxBrowser', () => ({
  default: ({ sessionId }: { sessionId: string }) => <div>Browser for {sessionId}</div>,
}))
vi.mock('../tasks/SpecTaskTerminalDrawer', () => ({
  default: ({ sessionId }: { sessionId: string }) => <div>Terminal for {sessionId}</div>,
}))
vi.mock('./OrgAgentSettingsPane', () => ({
  default: ({ sessionId, bot, instance }: { sessionId: string; bot: { id: string }; instance?: { runtime?: string } }) => (
    <div>Settings for {sessionId} of {bot.id}{instance ? ` as instance (${instance.runtime})` : ''}</div>
  ),
}))
vi.mock('../session/SubagentsPanel', () => ({
  default: ({ interactions }: { interactions: readonly unknown[] }) => <div>Subagents from {interactions.length} interactions</div>,
}))
vi.mock('react-resizable-panels', () => ({
  Group: ({ children, defaultLayout, onLayoutChange }: {
    children: ReactNode
    defaultLayout: Record<string, number>
    onLayoutChange: (layout: Record<string, number>) => void
  }) => <div data-testid="workspace-layout" data-layout={JSON.stringify(defaultLayout)}>
    <button onClick={() => onLayoutChange({
      'org-agent-session-chat': 45,
      'org-agent-session-desktop': 55,
    })}>Simulate split resize</button>
    {children}
  </div>,
  Panel: ({ children, panelRef, onResize }: {
    children: ReactNode
    panelRef?: { current: unknown }
    onResize?: (size: { asPercentage: number }) => void
  }) => {
    if (panelRef) {
      panelRef.current = {
        getSize: () => ({ asPercentage: 55 }),
        collapse: () => onResize?.({ asPercentage: 0 }),
        expand: () => onResize?.({ asPercentage: 55 }),
        resize: (size: string) => mocks.resizedTo(size),
      }
    }
    return <div>{children}</div>
  },
  Separator: () => <div />,
}))

const runningBot = {
  id: 'b-eng',
  status: 'running',
  sandbox_status: 'running',
  effective_sandbox_runtime: 'ubuntu-desktop',
} as const

describe('OrgAgentSessionWorkspace', () => {
  beforeEach(() => {
    mocks.isBigScreen = true
    mocks.resizedTo.mockReset()
    localStorage.clear()
  })

  it('shows chat and desktop together on larger screens', () => {
    render(
      <OrgAgentSessionWorkspace sessionId="session-one" organizationId="acme">
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    expect(screen.getByText('Session chat')).toBeInTheDocument()
    expect(screen.getByText('Desktop for session-one')).toBeInTheDocument()
  })

  it('collapses and restores both sides of the desktop split', () => {
    render(
      <OrgAgentSessionWorkspace sessionId="session-panels" organizationId="acme">
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Collapse task panel' }))
    expect(screen.getByRole('button', { name: 'Show task panel' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show task panel' }))
    expect(screen.getByRole('button', { name: 'Collapse chat panel' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Collapse chat panel' }))
    expect(screen.queryByText('Session chat')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restore split view' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Restore split view' }))
    expect(screen.getByText('Session chat')).toBeInTheDocument()
  })

  it('restores the closed task panel and its last expanded width after switching chats', () => {
    const first = render(
      <OrgAgentSessionWorkspace sessionId="session-one" organizationId="acme">
        <div>First chat</div>
      </OrgAgentSessionWorkspace>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Simulate split resize' }))
    expect(JSON.parse(localStorage.getItem('helix.orgAgentSession.layout.acme') || '{}'))
      .toEqual({ 'org-agent-session-chat': 45, 'org-agent-session-desktop': 55 })
    fireEvent.click(screen.getByRole('button', { name: 'Collapse task panel' }))
    expect(localStorage.getItem('helix.orgAgentSession.contentCollapsed.acme')).toBe('true')
    first.unmount()

    render(
      <OrgAgentSessionWorkspace sessionId="session-two" organizationId="acme">
        <div>Second chat</div>
      </OrgAgentSessionWorkspace>,
    )

    expect(screen.getByTestId('workspace-layout')).toHaveAttribute('data-layout', JSON.stringify({
      'org-agent-session-chat': 100,
      'org-agent-session-desktop': 0,
    }))
    expect(screen.getByRole('button', { name: 'Show task panel' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show task panel' }))
    expect(mocks.resizedTo).toHaveBeenLastCalledWith('55%')
    expect(localStorage.getItem('helix.orgAgentSession.contentCollapsed.acme')).toBe('false')
  })

  it('restores chat and terminal visibility after switching chats', () => {
    const first = render(
      <OrgAgentSessionWorkspace sessionId="session-one" organizationId="acme">
        <div>First chat</div>
      </OrgAgentSessionWorkspace>,
    )
    fireEvent.click(screen.getByRole('button', { name: /terminal/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Collapse chat panel' }))
    first.unmount()

    render(
      <OrgAgentSessionWorkspace sessionId="session-two" organizationId="acme">
        <div>Second chat</div>
      </OrgAgentSessionWorkspace>,
    )

    expect(screen.getByRole('button', { name: 'Restore split view' })).toBeInTheDocument()
    expect(screen.getByText('Terminal for session-two')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Restore split view' }))
    expect(screen.getByText('Second chat')).toBeInTheDocument()
    expect(localStorage.getItem('helix.orgAgentSession.chatCollapsed.acme')).toBe('false')
  })

  it('lets smaller screens switch between chat and desktop', () => {
    mocks.isBigScreen = false
    render(
      <OrgAgentSessionWorkspace sessionId="session-two" organizationId="acme">
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    // Chat is a tab on a phone-width layout; the default view is desktop.
    expect(screen.getByText('Desktop for session-two')).toBeInTheDocument()
    expect(screen.queryByText('Session chat')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /chat view/i }))

    expect(screen.getByText('Session chat')).toBeInTheDocument()
    expect(screen.queryByText('Desktop for session-two')).not.toBeInTheDocument()
  })

  it('switches between the diff, files, browser and details surfaces', () => {
    render(
      <OrgAgentSessionWorkspace sessionId="session-three" organizationId="acme" bot={runningBot as any}>
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    fireEvent.click(screen.getByRole('button', { name: /diff view/i }))
    expect(screen.getByText('changes for session-three')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /files view/i }))
    expect(screen.getByText('files for session-three')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /browser view/i }))
    expect(screen.getByText('Browser for session-three')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /settings view/i }))
    expect(screen.getByText('Settings for session-three of b-eng')).toBeInTheDocument()
  })

  it('shows subagents in the agents view instead of the desktop', () => {
    render(
      <OrgAgentSessionWorkspace
        sessionId="session-agents"
        organizationId="acme"
        bot={runningBot as any}
        subagentInteractions={[{ id: 'int-1' }, { id: 'int-2' }] as any}
      >
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    fireEvent.click(screen.getByRole('button', { name: /agents view/i }))
    expect(screen.getByText('Subagents from 2 interactions')).toBeInTheDocument()
    expect(screen.queryByText('Desktop for session-agents')).toBeNull()
  })

  // An instance has no bot of its own lifecycle, but its Settings view shows
  // the bot it belongs to, described as an instance.
  it('shows the parent bot settings for a bot instance', () => {
    render(
      <OrgAgentSessionWorkspace
        sessionId="session-instance"
        organizationId="acme"
        sessionSandbox={{ runtime: 'headless-ubuntu', state: 'running' }}
        instanceOf={runningBot as any}
      >
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    fireEvent.click(screen.getByRole('button', { name: /settings view/i }))
    expect(screen.getByText('Settings for session-instance of b-eng as instance (headless-ubuntu)')).toBeInTheDocument()
  })

  it('hides the desktop for a headless bot and opens on the diff view', () => {
    render(
      <OrgAgentSessionWorkspace
        sessionId="session-four"
        organizationId="acme"
        bot={{ ...runningBot, effective_sandbox_runtime: 'headless-ubuntu' } as any}
      >
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    expect(screen.queryByRole('button', { name: /desktop view/i })).not.toBeInTheDocument()
    expect(screen.getByText('changes for session-four')).toBeInTheDocument()
  })

  it('offers start for a stopped bot and stop for a running one', () => {
    const onStart = vi.fn()
    const onStop = vi.fn()
    const { rerender } = render(
      <OrgAgentSessionWorkspace
        sessionId="session-five"
        organizationId="acme"
        bot={{ ...runningBot, status: 'stopped', sandbox_status: 'stopped' } as any}
        onStart={onStart}
        onStop={onStop}
      >
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )
    fireEvent.click(screen.getByRole('button', { name: /^start/i }))
    expect(onStart).toHaveBeenCalledTimes(1)

    rerender(
      <OrgAgentSessionWorkspace
        sessionId="session-five"
        organizationId="acme"
        bot={runningBot as any}
        onStart={onStart}
        onStop={onStop}
      >
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )
    fireEvent.click(screen.getByRole('button', { name: /^stop/i }))
    expect(onStop).toHaveBeenCalledTimes(1)
  })

  it('opens the terminal drawer from the toolbar', () => {
    render(
      <OrgAgentSessionWorkspace sessionId="session-six" organizationId="acme" bot={runningBot as any}>
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )
    fireEvent.click(screen.getByRole('button', { name: /terminal/i }))
    expect(screen.getByText('Terminal for session-six')).toBeInTheDocument()
  })
})

describe('botSandboxIndicatorState', () => {
  it('maps agent and sandbox status onto the indicator', () => {
    expect(botSandboxIndicatorState(undefined)).toBe('running')
    expect(botSandboxIndicatorState({ status: 'running' } as any)).toBe('running')
    expect(botSandboxIndicatorState({ status: 'stopped', sandbox_status: 'pending' } as any)).toBe('starting')
    expect(botSandboxIndicatorState({ status: 'stopped', sandbox_status: 'stopped' } as any)).toBe('stopped')
  })
})
