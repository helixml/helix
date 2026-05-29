import { FC, useCallback, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import DeleteSweepIcon from '@mui/icons-material/DeleteSweep'
import PauseIcon from '@mui/icons-material/Pause'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import VerticalAlignBottomIcon from '@mui/icons-material/VerticalAlignBottom'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

interface Props {
  runnerId: string
  // tail is the number of historical lines hydra emits on connect. Default
  // matches the server-side default; raise for deeper context.
  tail?: number
  // height of the terminal area. Defaults to a roomy 480px to match the
  // SandboxTerminal component.
  height?: number | string
}

type ConnectionState = 'connecting' | 'open' | 'reconnecting' | 'closed' | 'error'

const reconnectDelayMs = 2000
const queueWhilePausedCap = 2000

// Build the WebSocket URL for the admin runner logs endpoint. Uses
// same-origin so cookies flow through; admin auth is enforced by the
// admin subrouter middleware.
function runnerLogsUrl(runnerId: string, tail: number): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}/api/v1/admin/runners/${encodeURIComponent(
    runnerId,
  )}/logs?tail=${tail}&follow=true`
}

// RunnerLogs streams a Runner's hydra-aggregated log buffer over a WebSocket
// and renders it in an xterm.js terminal. Lines arrive as JSON
// `{t: RFC3339, line: string}`. ANSI escape sequences in `line` are passed
// through to xterm verbatim, so hydra's own colored output keeps its
// formatting.
//
// Pause holds the terminal at its current scroll/content state and queues
// incoming lines into an in-memory buffer (capped at queueWhilePausedCap).
// Resume drains the queue into the terminal. The WebSocket stays open while
// paused — pausing is purely a render gate.
//
// Reconnect: on close or error, the component schedules a single reconnect
// attempt after reconnectDelayMs. If that also fails, status flips to
// `closed` and the user can manually reload by toggling pause off-then-on
// (handled by including pause in the effect's dependency list would be
// over-engineering for v1; instead we surface the state for a future manual
// "Reconnect" button).
const RunnerLogs: FC<Props> = ({ runnerId, tail = 500, height = 480 }) => {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | undefined>()
  const fitRef = useRef<FitAddon | undefined>()
  const wsRef = useRef<WebSocket | undefined>()
  const pausedRef = useRef<boolean>(false)
  const queueRef = useRef<string[]>([])
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>()
  const attemptRef = useRef<number>(0)

  const [status, setStatus] = useState<ConnectionState>('connecting')
  const [paused, setPaused] = useState<boolean>(false)
  const [queuedCount, setQueuedCount] = useState<number>(0)

  const writeLine = useCallback((line: string) => {
    const term = termRef.current
    if (!term) return
    term.writeln(line)
  }, [])

  const handleIncoming = useCallback(
    (line: string) => {
      if (pausedRef.current) {
        const q = queueRef.current
        q.push(line)
        if (q.length > queueWhilePausedCap) {
          q.splice(0, q.length - queueWhilePausedCap)
        }
        setQueuedCount(q.length)
        return
      }
      writeLine(line)
    },
    [writeLine],
  )

  const togglePause = useCallback(() => {
    setPaused((prev) => {
      const next = !prev
      pausedRef.current = next
      if (!next) {
        // Resuming: drain queue, then scroll to bottom.
        const q = queueRef.current
        for (const line of q) writeLine(line)
        queueRef.current = []
        setQueuedCount(0)
        termRef.current?.scrollToBottom()
      }
      return next
    })
  }, [writeLine])

  const clearTerminal = useCallback(() => {
    termRef.current?.clear()
    queueRef.current = []
    setQueuedCount(0)
  }, [])

  const scrollToBottom = useCallback(() => {
    termRef.current?.scrollToBottom()
  }, [])

  // Open/maintain the WebSocket. The terminal itself is set up once per
  // `runnerId` change; the WS lifecycle is nested inside so reconnect doesn't
  // tear down xterm.
  useEffect(() => {
    if (!containerRef.current || !runnerId) return

    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 12,
      theme: { background: '#000000' },
      convertEol: true,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 10000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    termRef.current = term
    fitRef.current = fit
    try {
      fit.fit()
    } catch {
      // ignore — first paint may not have laid out yet
    }

    const onResize = () => {
      try {
        fit.fit()
      } catch {
        // ignore
      }
    }
    window.addEventListener('resize', onResize)

    let cancelled = false

    const connect = () => {
      if (cancelled) return
      attemptRef.current += 1
      setStatus(attemptRef.current === 1 ? 'connecting' : 'reconnecting')

      const ws = new WebSocket(runnerLogsUrl(runnerId, tail))
      wsRef.current = ws

      ws.onopen = () => {
        if (cancelled) return
        setStatus('open')
        attemptRef.current = 0
      }
      ws.onmessage = (e) => {
        if (typeof e.data !== 'string') return
        try {
          const msg = JSON.parse(e.data) as { t?: string; line?: string }
          if (typeof msg.line === 'string') handleIncoming(msg.line)
        } catch {
          // Backend always emits JSON; if it ever sends raw text, render it
          // unparsed so we don't silently drop log output.
          handleIncoming(e.data)
        }
      }
      ws.onerror = () => {
        if (cancelled) return
        setStatus('error')
      }
      ws.onclose = () => {
        if (cancelled) return
        // One short retry. After that, status sticks at "closed" until the
        // user navigates away and back.
        if (attemptRef.current <= 1) {
          setStatus('reconnecting')
          reconnectTimerRef.current = setTimeout(connect, reconnectDelayMs)
        } else {
          setStatus('closed')
        }
      }
    }

    connect()

    return () => {
      cancelled = true
      window.removeEventListener('resize', onResize)
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current)
      wsRef.current?.close()
      term.dispose()
      termRef.current = undefined
      fitRef.current = undefined
      wsRef.current = undefined
      queueRef.current = []
      attemptRef.current = 0
    }
  }, [runnerId, tail, handleIncoming])

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
        <StatusChip status={status} />
        <Box sx={{ flexGrow: 1 }} />
        {paused && queuedCount > 0 && (
          <Typography variant="caption" color="text.secondary">
            {queuedCount} line{queuedCount !== 1 ? 's' : ''} queued
          </Typography>
        )}
        <Tooltip title={paused ? 'Resume' : 'Pause'}>
          <IconButton size="small" onClick={togglePause}>
            {paused ? <PlayArrowIcon fontSize="small" /> : <PauseIcon fontSize="small" />}
          </IconButton>
        </Tooltip>
        <Tooltip title="Scroll to bottom">
          <IconButton size="small" onClick={scrollToBottom}>
            <VerticalAlignBottomIcon fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title="Clear">
          <IconButton size="small" onClick={clearTerminal}>
            <DeleteSweepIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Stack>
      <Box
        ref={containerRef}
        sx={{
          flexGrow: 1,
          minHeight: height,
          backgroundColor: '#000000',
          borderRadius: 1,
          overflow: 'hidden',
        }}
      />
    </Box>
  )
}

const StatusChip: FC<{ status: ConnectionState }> = ({ status }) => {
  const props = (() => {
    switch (status) {
      case 'open':
        return { label: 'Live', color: 'success' as const }
      case 'connecting':
        return { label: 'Connecting…', color: 'default' as const }
      case 'reconnecting':
        return { label: 'Reconnecting…', color: 'warning' as const }
      case 'error':
        return { label: 'Error', color: 'error' as const }
      case 'closed':
        return { label: 'Disconnected', color: 'error' as const }
    }
  })()
  return <Chip size="small" label={props.label} color={props.color} />
}

export default RunnerLogs
