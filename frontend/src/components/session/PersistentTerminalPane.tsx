import { FC, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import ListItemIcon from '@mui/material/ListItemIcon'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { Copy, MessageSquareText } from 'lucide-react'
import { copyTextToClipboard } from '../../utils/clipboard'
import '@xterm/xterm/css/xterm.css'

interface Props {
  websocketUrl: string
  readOnly?: boolean
  active?: boolean
  onActivate?: () => void
  onExit?: (exitCode: number) => void
  onCopyToChat?: (text: string) => void
}

interface TerminalContextMenu {
  mouseX: number
  mouseY: number
  text: string
}

const PersistentTerminalPane: FC<Props> = ({
  websocketUrl,
  readOnly = false,
  active = false,
  onActivate,
  onExit,
  onCopyToChat,
}) => {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const onExitRef = useRef(onExit)
  const onCopyToChatRef = useRef(onCopyToChat)
  const [selectedText, setSelectedText] = useState('')
  const [contextMenu, setContextMenu] = useState<TerminalContextMenu | null>(null)

  useEffect(() => {
    onExitRef.current = onExit
  }, [onExit])

  useEffect(() => {
    onCopyToChatRef.current = onCopyToChat
  }, [onCopyToChat])

  useEffect(() => {
    if (!containerRef.current) return

    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      theme: { background: '#090909' },
      convertEol: true,
      cursorBlink: !readOnly,
      disableStdin: readOnly,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    fit.fit()

    term.attachCustomKeyEventHandler((event) => {
      const isCopyShortcut = event.type === 'keydown'
        && event.key.toLowerCase() === 'c'
        && (event.metaKey || event.ctrlKey)
        && !event.altKey

      if (!isCopyShortcut || !term.hasSelection()) return true

      event.preventDefault()
      void copyTextToClipboard(term.getSelection()).catch(() => {})
      return false
    })

    const selectionDisposable = term.onSelectionChange(() => {
      setSelectedText(term.getSelection())
    })

    const ws = new WebSocket(websocketUrl)
    ws.binaryType = 'arraybuffer'

    const sendResize = () => {
      if (ws.readyState !== WebSocket.OPEN) return
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    }

    ws.onopen = () => {
      sendResize()
      if (!readOnly && active) term.focus()
    }
    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        try {
          const message = JSON.parse(event.data)
          if (message?.type === 'error') {
            term.write(`\r\n\x1b[31m${message.message}\x1b[0m\r\n`)
          }
          if (message?.type === 'exit') {
            onExitRef.current?.(Number(message.code) || 0)
          }
        } catch {
          term.write(event.data)
        }
        return
      }
      term.write(new Uint8Array(event.data as ArrayBuffer))
    }
    ws.onerror = () => {
      term.write('\r\n\x1b[31mTerminal connection error\x1b[0m\r\n')
    }
    ws.onclose = () => {
      term.write('\r\n\x1b[33mDisconnected.\x1b[0m\r\n')
    }

    const dataDisposable = readOnly
      ? undefined
      : term.onData((data) => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(new TextEncoder().encode(data))
          }
        })

    let resizeFrame: number | undefined
    const handleResize = () => {
      if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame)
      resizeFrame = window.requestAnimationFrame(() => {
        resizeFrame = undefined
        try {
          fit.fit()
          sendResize()
        } catch {
          // A resize may already be queued while the pane is being removed.
        }
      })
    }
    window.addEventListener('resize', handleResize)
    const resizeObserver = typeof ResizeObserver === 'undefined'
      ? undefined
      : new ResizeObserver(handleResize)
    resizeObserver?.observe(containerRef.current)

    return () => {
      window.removeEventListener('resize', handleResize)
      resizeObserver?.disconnect()
      if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame)
      dataDisposable?.dispose()
      selectionDisposable.dispose()
      ws.close()
      term.dispose()
    }
    // `active` only controls which newly mounted split receives focus. Pane
    // activation afterward comes from the user's pointer event and must not
    // reconnect the WebSocket.
  }, [readOnly, websocketUrl])

  return (
    <Box
      onMouseDown={onActivate}
      onContextMenu={(event) => {
        if (!selectedText) return
        event.preventDefault()
        event.stopPropagation()
        setContextMenu({
          mouseX: event.clientX,
          mouseY: event.clientY,
          text: selectedText,
        })
      }}
      sx={{
        position: 'relative',
        minWidth: 0,
        minHeight: 0,
        width: '100%',
        height: '100%',
        bgcolor: '#090909',
        outline: active ? '1px solid rgba(255, 255, 255, 0.16)' : 'none',
        outlineOffset: -1,
        p: 0.75,
      }}
    >
      <Box ref={containerRef} sx={{ width: '100%', height: '100%' }} />
      <Menu
        open={contextMenu !== null}
        onClose={() => setContextMenu(null)}
        anchorReference="anchorPosition"
        anchorPosition={contextMenu
          ? { top: contextMenu.mouseY, left: contextMenu.mouseX }
          : undefined}
        MenuListProps={{
          dense: true,
          'aria-label': 'Terminal selection actions',
          sx: { py: 0.5 },
        }}
        slotProps={{
          paper: {
            sx: {
              minWidth: 150,
              border: '1px solid',
              borderColor: 'divider',
              boxShadow: 4,
            },
          },
        }}
      >
        <MenuItem
          onClick={() => {
            if (contextMenu) {
              void copyTextToClipboard(contextMenu.text).catch(() => {})
            }
            setContextMenu(null)
          }}
        >
          <ListItemIcon sx={{ minWidth: 30 }}>
            <Copy size={16} />
          </ListItemIcon>
          Copy
        </MenuItem>
        {onCopyToChat && (
          <MenuItem
            onClick={() => {
              if (contextMenu) {
                onCopyToChatRef.current?.(contextMenu.text)
              }
              setContextMenu(null)
            }}
          >
            <ListItemIcon sx={{ minWidth: 30 }}>
              <MessageSquareText size={16} />
            </ListItemIcon>
            Add to chat
          </MenuItem>
        )}
      </Menu>
    </Box>
  )
}

export default PersistentTerminalPane
