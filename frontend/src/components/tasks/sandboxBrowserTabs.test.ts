import { describe, expect, it } from 'vitest'

import { closeSandboxBrowserTabs } from './sandboxBrowserTabs'

describe('closeSandboxBrowserTabs', () => {
  const tabs = ['first', 'second', 'third', 'fourth']

  it('closes one tab and selects the following tab', () => {
    expect(closeSandboxBrowserTabs(tabs, 'second', 'second', 'close')).toEqual({
      activeTabId: 'third',
      openTabIds: ['first', 'third', 'fourth'],
    })
  })

  it('closes other tabs and selects the target', () => {
    expect(closeSandboxBrowserTabs(tabs, 'first', 'third', 'close_others')).toEqual({
      activeTabId: 'third',
      openTabIds: ['third'],
    })
  })

  it('closes tabs to the right and keeps an active tab on the left', () => {
    expect(closeSandboxBrowserTabs(tabs, 'first', 'second', 'close_right')).toEqual({
      activeTabId: 'first',
      openTabIds: ['first', 'second'],
    })
  })

  it('closes all tabs', () => {
    expect(closeSandboxBrowserTabs(tabs, 'third', 'third', 'close_all')).toEqual({
      activeTabId: null,
      openTabIds: [],
    })
  })
})
