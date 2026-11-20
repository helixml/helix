import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ProjectChatBotEntry } from './ProjectChatBotsGroup'
import type { SidebarBot } from './ProjectChatSidebar.logic'
import { sidebarTreeLabelInset } from './SidebarTree'

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../../hooks/useIsPhone', () => ({
  default: () => false,
}))

vi.mock('../../services/specTaskService', () => ({
  useSpecTasks: () => ({ data: [], isLoading: false, isFetching: false, isError: false }),
}))

const botInstances = vi.hoisted(() => ({ data: [] as Array<Record<string, string>> }))
vi.mock('../../services/helixOrgService', () => ({
  useBotInstances: () => ({ data: botInstances.data, isLoading: false, isFetching: false, isError: false }),
}))

vi.mock('../../hooks/useApps', () => ({
  default: () => ({ apps: [] }),
}))

const renderBot = (bot: SidebarBot, activeItemId = '', collapsed = true, onOpenMenu = vi.fn()) => render(
  <ProjectChatBotEntry
    orgId="org-one"
    bot={bot}
    collapsed={collapsed}
    busy={false}
    projects={[]}
    query=""
    activeItemId={activeItemId}
    relativeTimeNow={Date.now()}
    enabled
    organizationMembers={[]}
    archivingItemId={null}
    onToggle={vi.fn()}
    onOpen={vi.fn()}
    onOpenMenu={onOpenMenu}
    onOpenItem={vi.fn()}
    onOpenItemContextMenu={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

describe('ProjectChatBotEntry', () => {
  // Touch screens have no right-click, so the bot menu (new instances,
  // settings) must be reachable from a visible button.
  it('opens the bot menu from its actions button', () => {
    const onOpenMenu = vi.fn()
    renderBot({ id: 'chief', name: 'Chief of Staff', running: true, activity: null, restartRequired: false }, '', true, onOpenMenu)

    fireEvent.click(screen.getByRole('button', { name: 'More actions for Chief of Staff' }))
    expect(onOpenMenu).toHaveBeenCalledTimes(1)
  })

  it('shows working as a pulsing activity dot', () => {
    const { container } = renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      activity: { label: 'Working', color: '#34d399', working: true },
      restartRequired: false,
      portraitIndex: 0,
    })

    const workingDot = screen.getByRole('status', { name: 'Working' })
    expect(workingDot).toHaveStyle({ backgroundColor: '#34d399' })
    expect(workingDot).not.toHaveStyle({ animation: 'none' })
    expect(workingDot).not.toHaveTextContent('Working')
    expect(container.querySelector('img')).toHaveStyle({ filter: 'grayscale(1)' })
    expect(screen.getByTestId('sidebar-bot-trailing-slot')).toContainElement(screen.getByRole('button', { name: 'More actions for Chief of Staff' }))
  })

  it('shows a running but idle bot with a quiet gray dot', () => {
    renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      activity: { label: 'Idle', color: '#a1a1aa', working: false },
      restartRequired: false,
    })

    expect(screen.getByRole('status', { name: 'Idle' })).toHaveStyle({
      backgroundColor: '#a1a1aa',
      animation: 'none',
    })
  })

  it('keeps activity visible when selected', () => {
    renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      activity: { label: 'Working', color: '#34d399', working: true },
      restartRequired: false,
      sessionId: 'ses-chief',
    }, 'ses-chief')

    expect(screen.getByRole('status', { name: 'Working' })).toBeInTheDocument()
  })

  it('lists instances under the bot with a delete action and sandbox status', () => {
    botInstances.data = [{
      session_id: 'ses_instance',
      name: 'Broker for ACME',
      sandbox_status: 'running',
      created_at: '2026-09-24T12:00:00Z',
      updated_at: '2026-09-24T12:05:00Z',
    }]
    const { container } = renderBot({
      id: 'b-broker',
      name: 'Broker',
      running: false,
      activity: { label: 'Offline', color: '#a1a1aa', working: false },
      restartRequired: false,
    }, '', false)

    expect(screen.getByText('Broker for ACME')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete instance Broker for ACME' })).toBeInTheDocument()
    expect(container.querySelector('[data-instance-status="running"]')).toBeInTheDocument()
    botInstances.data = []
  })

  it('shows the instance count on a collapsed bot', () => {
    botInstances.data = [
      { session_id: 'ses_1', name: 'One', sandbox_status: 'running' },
      { session_id: 'ses_2', name: 'Two', sandbox_status: '' },
    ]
    renderBot({ id: 'b-broker', name: 'Broker', running: false, activity: { label: 'Offline', color: '#a1a1aa', working: false }, restartRequired: false })

    expect(screen.getByTestId('sidebar-bot-instance-count')).toHaveTextContent('2')
    expect(screen.queryByText('One')).toBeNull()
    botInstances.data = []
  })

  // A bot's tasks and instances must read as nested inside it: they start on
  // the bot name's column, never under its chevron or portrait.
  it('indents its rows to the bot name column whether or not the bot shows activity', () => {
    botInstances.data = [{ session_id: 'ses_instance', name: 'Broker for ACME', sandbox_status: 'running' }]
    const { container } = renderBot({ id: 'b-broker', name: 'Broker', running: false, activity: null, restartRequired: false }, '', false)

    const children = screen.getByTestId('sidebar-tree-children')
    // Child rows add their own 8px padding on top of the group indent.
    expect(children).toHaveStyle({ paddingLeft: `${sidebarTreeLabelInset(false) - 8}px` })
    expect(children).toContainElement(screen.getByText('Broker for ACME'))
    // The activity dot is badged on the portrait, never a column of its own.
    expect(container.querySelectorAll('.project-chat-agent > *')[1]).toBe(screen.getByTestId('sidebar-tree-icon-slot'))
    botInstances.data = []
  })
})
