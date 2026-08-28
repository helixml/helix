import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PersistentTerminalPane from './PersistentTerminalPane'

const clipboard = vi.hoisted(() => ({
  copyTextToClipboard: vi.fn(() => Promise.resolve()),
}))

vi.mock('../../utils/clipboard', () => clipboard)

const xterm = vi.hoisted(() => {
  const state: {
    keyHandler?: (event: KeyboardEvent) => boolean
    selectionHandler?: () => void
    selection: string
  } = {
    selection: '',
  }

  const terminal = {
    cols: 80,
    rows: 24,
    loadAddon: vi.fn(),
    open: vi.fn(),
    focus: vi.fn(),
    write: vi.fn(),
    dispose: vi.fn(),
    onData: vi.fn(() => ({ dispose: vi.fn() })),
    attachCustomKeyEventHandler: vi.fn((handler: (event: KeyboardEvent) => boolean) => {
      state.keyHandler = handler
    }),
    onSelectionChange: vi.fn((handler: () => void) => {
      state.selectionHandler = handler
      return { dispose: vi.fn() }
    }),
    hasSelection: vi.fn(() => state.selection.length > 0),
    getSelection: vi.fn(() => state.selection),
    clearSelection: vi.fn(() => {
      state.selection = ''
      state.selectionHandler?.()
    }),
  }

  return { state, terminal }
})

vi.mock('@xterm/xterm', () => ({
  Terminal: vi.fn(function Terminal() {
    return xterm.terminal
  }),
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: vi.fn(function FitAddon() {
    return { fit: vi.fn() }
  }),
}))

class MockWebSocket {
  static OPEN = 1
  readyState = MockWebSocket.OPEN
  binaryType = ''
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  send = vi.fn()
  close = vi.fn()
}

describe('PersistentTerminalPane', () => {
  beforeEach(() => {
    xterm.state.selection = ''
    xterm.state.keyHandler = undefined
    xterm.state.selectionHandler = undefined
    vi.clearAllMocks()
    vi.stubGlobal('WebSocket', MockWebSocket)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('copies a selection with Ctrl+C but leaves Ctrl+C to the terminal without one', () => {
    render(<PersistentTerminalPane websocketUrl="ws://terminal" />)

    xterm.state.selection = 'selected output'
    const copyEvent = new KeyboardEvent('keydown', {
      key: 'c',
      ctrlKey: true,
      cancelable: true,
    })

    expect(xterm.state.keyHandler?.(copyEvent)).toBe(false)
    expect(clipboard.copyTextToClipboard).toHaveBeenCalledWith('selected output')

    const macCopyEvent = new KeyboardEvent('keydown', {
      key: 'c',
      metaKey: true,
      cancelable: true,
    })

    expect(xterm.state.keyHandler?.(macCopyEvent)).toBe(false)
    expect(clipboard.copyTextToClipboard).toHaveBeenLastCalledWith('selected output')

    xterm.state.selection = ''
    const interruptEvent = new KeyboardEvent('keydown', {
      key: 'c',
      ctrlKey: true,
      cancelable: true,
    })

    expect(xterm.state.keyHandler?.(interruptEvent)).toBe(true)
    expect(clipboard.copyTextToClipboard).toHaveBeenCalledTimes(2)
  })

  it('offers copy and add-to-chat actions from the selection context menu', () => {
    const onCopyToChat = vi.fn()
    const { container } = render(
      <PersistentTerminalPane
        websocketUrl="ws://terminal"
        onCopyToChat={onCopyToChat}
      />,
    )

    act(() => {
      xterm.state.selection = 'build failed on line 42'
      xterm.state.selectionHandler?.()
    })

    expect(screen.queryByRole('button', { name: 'Copy to chat' })).not.toBeInTheDocument()

    fireEvent.contextMenu(container.firstElementChild!, { clientX: 120, clientY: 80 })
    fireEvent.click(screen.getByRole('menuitem', { name: 'Copy' }))
    expect(clipboard.copyTextToClipboard).toHaveBeenCalledWith('build failed on line 42')

    fireEvent.contextMenu(container.firstElementChild!, { clientX: 120, clientY: 80 })
    fireEvent.click(screen.getByRole('menuitem', { name: 'Add to chat' }))
    expect(onCopyToChat).toHaveBeenCalledWith('build failed on line 42')
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })
})
