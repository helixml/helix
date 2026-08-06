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

  it('creates groups and independent split panes', () => {
    const initial = createTerminalLayout('one')
    const split = splitActiveTerminal(initial, 'two', 'vertical')
    const grouped = addTerminalGroup(split, 'three')

    expect(grouped.groups).toEqual([
      { id: 'group-one', paneNames: ['one', 'two'], direction: 'vertical' },
      { id: 'group-three', paneNames: ['three'], direction: 'horizontal' },
    ])
    expect(grouped.activePaneName).toBe('three')
  })

  it('collapses panes and groups as sessions are removed', () => {
    const split = splitActiveTerminal(createTerminalLayout('one'), 'two', 'horizontal')
    const afterPane = removeTerminalPane(split, 'two')
    const afterGroup = removeTerminalPane(afterPane, 'one')

    expect(afterPane.groups[0].paneNames).toEqual(['one'])
    expect(afterGroup.groups).toEqual([])
    expect(afterGroup.activePaneName).toBeNull()
  })
})
