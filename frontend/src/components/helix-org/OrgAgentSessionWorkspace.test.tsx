import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrgAgentSessionWorkspace, { botSandboxIndicatorState } from './OrgAgentSessionWorkspace'

const mocks = vi.hoisted(() => ({
  isBigScreen: true,
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
  default: ({ sessionId }: { sessionId: string }) => <div>Settings for {sessionId}</div>,
}))
vi.mock('react-resizable-panels', () => ({
  Group: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Panel: ({ children, panelRef }: {
    children: ReactNode
    panelRef?: { current: unknown }
  }) => {
    if (panelRef) {
      panelRef.current = {
        getSize: () => ({ asPercentage: 62 }),
        collapse: () => {},
        expand: () => {},
        resize: () => {},
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
    expect(screen.getByText('Settings for session-three')).toBeInTheDocument()
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
