import { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select, { SelectChangeEvent } from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

import {
  sandboxTerminalUrl,
  useSandboxTerminalSessions,
} from '../../services/sandboxesService'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
  // height of the rendered terminal — defaults to a roomy value for the tab
  // view, callers (e.g. the card preview) can pass a smaller value.
  height?: number | string
  // showControls renders a small toolbar with the session selector and a
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
//
// When showControls is on, the toolbar lists every existing tmux session in
// the container so the user can switch between them. A "New session" button
// creates a fresh one.
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

  // Poll the existing tmux sessions inside the sandbox so the switcher stays
  // current as new sessions are created from elsewhere (e.g. a second browser
  // tab). Only enabled while the sandbox is running and controls are shown.
  const sessionsQuery = useSandboxTerminalSessions(
    showControls && running ? orgId : undefined,
    showControls && running ? sandboxId : undefined,
  )

  // Build the dropdown options: every helix-managed session reported by the
  // backend, plus the locally selected one if it hasn't been observed yet
  // (which is the normal case immediately after "New session" — tmux only
  // materialises the session when the websocket connects).
  const sessionOptions = useMemo<string[]>(() => {
    const set = new Set<string>()
    set.add(sessionName)
    for (const s of sessionsQuery.data?.sessions ?? []) {
      if (s?.name) set.add(s.name)
    }
    return Array.from(set)
  }, [sessionName, sessionsQuery.data])

  const handleNewSession = useCallback(() => {
    const fresh = generateSessionName()
    writeStoredSession(sandboxId, fresh)
    setSessionName(fresh)
  }, [sandboxId])

  const handleSelectSession = useCallback(
    (e: SelectChangeEvent<string>) => {
      const next = e.target.value
      if (!next || next === sessionName) return
      writeStoredSession(sandboxId, next)
      setSessionName(next)
    },
    [sandboxId, sessionName],
  )

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
        <Stack direction="row" alignItems="center" spacing={1.5} sx={{ minHeight: 40 }}>
          <FormControl size="small" sx={{ minWidth: 240 }}>
            <InputLabel id={`sandbox-${sandboxId}-session-label`}>Session</InputLabel>
            <Select
              labelId={`sandbox-${sandboxId}-session-label`}
              label="Session"
              value={sessionName}
              onChange={handleSelectSession}
              renderValue={(v) => `helix-${v}`}
            >
              {sessionOptions.map((name) => {
                const meta = sessionsQuery.data?.sessions?.find((s) => s.name === name)
                return (
                  <MenuItem key={name} value={name}>
                    <Stack direction="row" alignItems="center" spacing={1}>
                      <Box component="span" sx={{ fontFamily: 'monospace' }}>
                        helix-{name}
                      </Box>
                      {meta?.attached && (
                        <Typography variant="caption" color="text.secondary">
                          (attached)
                        </Typography>
                      )}
                      {!meta && (
                        <Typography variant="caption" color="text.secondary">
                          (new)
                        </Typography>
                      )}
                    </Stack>
                  </MenuItem>
                )
              })}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary">
            Reconnects reattach to the selected tmux session.
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Tooltip title="Start a fresh tmux session in this sandbox">
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
