import { describe, expect, it } from 'vitest'

import {
  addTerminalGroup,
  createTerminalLayout,
  readTerminalLayout,
  removeTerminalPane,
  splitActiveTerminal,
} from './sessionTerminalLayout'

describe('session terminal layout', () => {
  it('migrates the previous single-session storage value', () => {
    expect(readTerminalLayout('abc123', 'fallback')).toEqual(createTerminalLayout('abc123'))
  })

  it('migrates the flat version 1 layout', () => {
    const legacyLayout = JSON.stringify({
      version: 1,
      groups: [{
        id: 'group-one',
        paneNames: ['one', 'two'],
        direction: 'vertical',
      }],
      activeGroupId: 'group-one',
      activePaneName: 'two',
    })

    expect(readTerminalLayout(legacyLayout, 'fallback')).toEqual({
      version: 2,
      groups: [{
        id: 'group-one',
        root: {
          type: 'split',
          direction: 'vertical',
          children: [
            { type: 'pane', sessionName: 'one' },
            { type: 'pane', sessionName: 'two' },
          ],
        },
      }],
      activeGroupId: 'group-one',
      activePaneName: 'two',
    })
  })

  it('creates a new terminal after the persisted layout was emptied', () => {
    const emptyLayout = JSON.stringify({
      version: 2,
      groups: [],
      activeGroupId: null,
      activePaneName: null,
    })

    expect(readTerminalLayout(emptyLayout, 'new-session')).toEqual(
      createTerminalLayout('new-session'),
    )
  })

  it('splits only the active pane', () => {
    const horizontal = splitActiveTerminal(
      createTerminalLayout('one'),
      'two',
      'horizontal',
    )
    const vertical = splitActiveTerminal(
      { ...horizontal, activePaneName: 'one' },
      'three',
      'vertical',
    )

    expect(vertical.groups[0].root).toEqual({
      type: 'split',
      direction: 'horizontal',
      children: [
        {
          type: 'split',
          direction: 'vertical',
          children: [
            { type: 'pane', sessionName: 'one' },
            { type: 'pane', sessionName: 'three' },
          ],
        },
        { type: 'pane', sessionName: 'two' },
      ],
    })
    expect(vertical.activePaneName).toBe('three')
  })

  it('creates independent terminal groups', () => {
    const split = splitActiveTerminal(createTerminalLayout('one'), 'two', 'vertical')
    const grouped = addTerminalGroup(split, 'three')

    expect(grouped.groups[1]).toEqual({
      id: 'group-three',
      root: { type: 'pane', sessionName: 'three' },
    })
    expect(grouped.activePaneName).toBe('three')
  })

  it('collapses nested splits and groups as sessions are removed', () => {
    const horizontal = splitActiveTerminal(
      createTerminalLayout('one'),
      'two',
      'horizontal',
    )
    const nested = splitActiveTerminal(
      { ...horizontal, activePaneName: 'one' },
      'three',
      'vertical',
    )
    const afterNestedPane = removeTerminalPane(nested, 'three')
    const afterSibling = removeTerminalPane(afterNestedPane, 'two')
    const afterGroup = removeTerminalPane(afterSibling, 'one')

    expect(afterNestedPane.groups[0].root).toEqual({
      type: 'split',
      direction: 'horizontal',
      children: [
        { type: 'pane', sessionName: 'one' },
        { type: 'pane', sessionName: 'two' },
      ],
    })
    expect(afterSibling.groups[0].root).toEqual({
      type: 'pane',
      sessionName: 'one',
    })
    expect(afterGroup.groups).toEqual([])
    expect(afterGroup.activePaneName).toBeNull()
  })
})
