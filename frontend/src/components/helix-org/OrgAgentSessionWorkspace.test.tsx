import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrgAgentSessionWorkspace from './OrgAgentSessionWorkspace'

const mocks = vi.hoisted(() => ({
  isBigScreen: true,
}))

vi.mock('../../hooks/useIsBigScreen', () => ({ default: () => mocks.isBigScreen }))
vi.mock('../../hooks/useLightTheme', () => ({ default: () => ({ isLight: true }) }))
vi.mock('../external-agent/ExternalAgentDesktopViewer', () => ({
  default: ({ sessionId }: { sessionId: string }) => <div>Desktop for {sessionId}</div>,
}))
vi.mock('react-resizable-panels', () => ({
  Group: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Panel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Separator: () => <div />,
}))

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

  it('lets smaller screens switch between chat and desktop', () => {
    mocks.isBigScreen = false
    render(
      <OrgAgentSessionWorkspace sessionId="session-two" organizationId="acme">
        <div>Session chat</div>
      </OrgAgentSessionWorkspace>,
    )

    expect(screen.getByText('Session chat')).toBeInTheDocument()
    expect(screen.queryByText('Desktop for session-two')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /desktop/i }))

    expect(screen.getByText('Desktop for session-two')).toBeInTheDocument()
    expect(screen.queryByText('Session chat')).not.toBeInTheDocument()
  })
})
