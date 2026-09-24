import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ProjectChatBotEntry } from './ProjectChatBotsGroup'
import type { SidebarBot } from './ProjectChatSidebar.logic'

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../../hooks/useIsPhone', () => ({
  default: () => false,
}))

vi.mock('../../services/specTaskService', () => ({
  useSpecTasks: () => ({ data: [], isLoading: false, isFetching: false, isError: false }),
}))

const renderBot = (bot: SidebarBot, activeItemId = '') => render(
  <ProjectChatBotEntry
    orgId="org-one"
    bot={bot}
    collapsed
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
    onOpenSettings={vi.fn()}
    onOpenMenu={vi.fn()}
    onOpenItem={vi.fn()}
    onOpenItemContextMenu={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

describe('ProjectChatBotEntry', () => {
  it('keeps the online dot with a trailing working label', () => {
    const { container } = renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      working: true,
      restartRequired: false,
    })

    const trailingSlot = screen.getByTestId('sidebar-bot-trailing-slot')
    const workingLabel = screen.getByRole('status', { name: 'Working' })
    expect(workingLabel).toHaveTextContent('Working')
    expect(workingLabel).toHaveStyle({ color: '#34d399' })
    expect(trailingSlot).toContainElement(workingLabel)
    expect(trailingSlot).toContainElement(screen.getByRole('button', { name: 'Settings for Chief of Staff' }))
    expect(container.querySelector('[data-bot-status="running"]')).toBeInTheDocument()
  })

  it('keeps the presence dot when the bot is idle', () => {
    const { container } = renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      working: false,
      restartRequired: false,
    })

    expect(screen.queryByRole('status', { name: 'Working' })).toBeNull()
    expect(container.querySelector('[data-bot-status="running"]')).toBeInTheDocument()
  })

  it.each([
    ['bot id', 'chief'],
    ['session id', 'ses-chief'],
  ])('hides the working indicator when selected by %s', (_, activeItemId) => {
    const { container } = renderBot({
      id: 'chief',
      name: 'Chief of Staff',
      running: true,
      working: true,
      restartRequired: false,
      sessionId: 'ses-chief',
    }, activeItemId)

    expect(screen.queryByRole('status', { name: 'Working' })).toBeNull()
    expect(container.querySelector('[data-bot-status="running"]')).toBeInTheDocument()
  })
})
