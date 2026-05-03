import { FC, useCallback, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

import { sandboxTerminalUrl } from '../../services/sandboxesService'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
  // height of the rendered terminal — defaults to a roomy value for the tab
  // view, callers (e.g. the card preview) can pass a smaller value.
  height?: number | string
  // showControls renders a small toolbar with the active session name and a
  // "New session" button. Disabled for compact previews.
  showControls?: boolean
  // readOnly suppresses keyboard input — used for the read-only card preview
  // so accidental clicks don't run shell commands.
  readOnly?: boolean
  // fillContainer makes the component stretch to its parent's height instead
  // of sizing to its `height` prop. Used when the parent already constrains
  // height (e.g. an aspect-ratio box on a card).
  fillContainer?: boolean
}

const sessionStorageKey = (sandboxId: string) => `helix.sandbox.${sandboxId}.terminalSession`

const generateSessionName = (): string => {
  // crypto.randomUUID is available in modern browsers; fall back to a
  // timestamp+random string for older environments.
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID().replace(/-/g, '').slice(0, 12)
  }
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
}

const readStoredSession = (sandboxId: string): string | undefined => {
  try {
    const v = window.localStorage.getItem(sessionStorageKey(sandboxId))
    return v || undefined
  } catch {
    return undefined
  }
}

const writeStoredSession = (sandboxId: string, name: string) => {
  try {
    window.localStorage.setItem(sessionStorageKey(sandboxId), name)
  } catch {
    // ignore — best-effort persistence
  }
}

// SandboxTerminal renders an xterm.js terminal connected to the sandbox via
// a websocket. The shell is wrapped server-side in `tmux new-session -A -s
// helix-<sessionName>`, so reconnects (page refresh, ws drop) reattach to the
// same tmux session — preserving working dir, scrollback, and any in-flight
// processes. The session name is persisted in localStorage per-sandbox.
const SandboxTerminal: FC<Props> = ({
  orgId,
  sandboxId,
  running,
  height = 480,
  showControls = true,
  readOnly = false,
  fillContainer = false,
}) => {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | undefined>()
  const fitRef = useRef<FitAddon | undefined>()
  const wsRef = useRef<WebSocket | undefined>()

  const [sessionName, setSessionName] = useState<string>(() => {
    const stored = readStoredSession(sandboxId)
    if (stored) return stored
    const fresh = generateSessionName()
    writeStoredSession(sandboxId, fresh)
    return fresh
  })

  const handleNewSession = useCallback(() => {
    const fresh = generateSessionName()
    writeStoredSession(sandboxId, fresh)
    setSessionName(fresh)
  }, [sandboxId])

  useEffect(() => {
    if (!running || !containerRef.current) return
    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      theme: { background: '#000000' },
      convertEol: true,
      cursorBlink: !readOnly,
      disableStdin: readOnly,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    termRef.current = term
    fitRef.current = fit
    fit.fit()

    const ws = new WebSocket(sandboxTerminalUrl(orgId, sandboxId, sessionName))
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    const sendResize = () => {
      if (ws.readyState !== WebSocket.OPEN) return
      ws.send(
        JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows })
      )
    }

    ws.onopen = () => {
      sendResize()
      if (!readOnly) term.focus()
    }
    ws.onmessage = (e) => {
      if (typeof e.data === 'string') {
        try {
          const msg = JSON.parse(e.data)
          if (msg?.type === 'error') {
            term.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`)
          }
        } catch {
          term.write(e.data)
        }
      } else {
        term.write(new Uint8Array(e.data as ArrayBuffer))
      }
    }
    ws.onerror = () => {
      term.write('\r\n\x1b[31mTerminal connection error\x1b[0m\r\n')
    }
    ws.onclose = () => {
      term.write('\r\n\x1b[33mDisconnected.\x1b[0m\r\n')
    }

    if (!readOnly) {
      term.onData((data: string) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(new TextEncoder().encode(data))
        }
      })
    }

    const onResize = () => {
      try {
        fit.fit()
        sendResize()
      } catch {
        // ignore
      }
    }
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      ws.close()
      term.dispose()
      termRef.current = undefined
      fitRef.current = undefined
      wsRef.current = undefined
    }
    // sessionName is intentionally in the deps so a "New session" click
    // tears down the old WS + xterm and reconnects against the new tmux session.
  }, [orgId, sandboxId, running, sessionName, readOnly])

  if (!running) {
    return (
      <Box
        sx={{
          p: 4,
          textAlign: 'center',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          Sandbox is not running yet — terminal will be available when status is "running".
        </Typography>
      </Box>
    )
  }

  return (
    <Stack
      spacing={1}
      sx={{
        width: '100%',
        ...(fillContainer ? { height: '100%' } : {}),
      }}
    >
      {showControls && (
        <Stack direction="row" alignItems="center" spacing={1} sx={{ minHeight: 28 }}>
          <Typography variant="caption" color="text.secondary">
            Session{' '}
            <Box
              component="span"
              sx={{ fontFamily: 'monospace', color: 'text.primary' }}
            >
              helix-{sessionName}
            </Box>
            {' '}— reconnects reattach to this tmux session.
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Tooltip title="Discard the current tmux session and start a fresh one">
            <Button
              size="small"
              variant="outlined"
              startIcon={<RestartAltIcon fontSize="small" />}
              onClick={handleNewSession}
            >
              New session
            </Button>
          </Tooltip>
        </Stack>
      )}
      <Box
        sx={{
          p: 1,
          bgcolor: '#000',
          ...(fillContainer
            ? { flex: 1, minHeight: 0 }
            : {
                height,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
              }),
        }}
      >
        <Box ref={containerRef} sx={{ width: '100%', height: '100%' }} />
      </Box>
    </Stack>
  )
}

export default SandboxTerminal
