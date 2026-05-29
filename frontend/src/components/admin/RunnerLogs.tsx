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
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord'
import SyncIcon from '@mui/icons-material/Sync'
import ErrorIcon from '@mui/icons-material/Error'
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
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
  // showOpenInNewTab renders an "Open in new tab" icon button in the
  // toolbar that opens /admin/runner-logs/:runner_id as a standalone
  // full-screen page. Hide when the component is already rendered as
  // that standalone page (otherwise the user can recursively pop out).
  showOpenInNewTab?: boolean
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
// Resume drains the queue into the terminal in one batched write. The
// WebSocket stays open while paused — pausing is purely a render gate.
//
// Reconnect: one short retry on close. After it also fails, status flips
// to `closed` until the user navigates away and back. The retry is gated
// by a one-shot `hasRetriedRef` flag (separate from the
// connecting-vs-reconnecting state) so a flapping connection doesn't loop
// forever resetting itself on every successful onopen.
const RunnerLogs: FC<Props> = ({
  runnerId,
  tail = 500,
  height = 480,
  showOpenInNewTab = true,
}) => {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const pausedRef = useRef<boolean>(false)
  const queueRef = useRef<string[]>([])
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const hasRetriedRef = useRef<boolean>(false)
  // handleIncomingRef lets the WS handlers call the latest write logic
  // without re-creating the WS effect on every render. The WS effect only
  // depends on `runnerId` and `tail` — see the long comment in the effect.
  const handleIncomingRef = useRef<(line: string) => void>(() => {})

  const [status, setStatus] = useState<ConnectionState>('connecting')
  const [paused, setPaused] = useState<boolean>(false)
  const [queuedCount, setQueuedCount] = useState<number>(0)

  // Refresh handleIncomingRef on every render so the WS callbacks see
  // current state (paused flag, write function). The actual handler is
  // bound to refs only, so this is cheap.
  handleIncomingRef.current = useCallback(
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
      termRef.current?.writeln(line)
    },
    [],
  )

  const togglePause = useCallback(() => {
    setPaused((prev) => {
      const next = !prev
      pausedRef.current = next
      if (!next) {
        // Resuming. Batch the queued lines into a single xterm.write call
        // (~one render commit) so large drains don't block the main
        // thread the way per-line writelns of 2000 lines would.
        const q = queueRef.current
        if (q.length > 0) {
          termRef.current?.write(q.join('\r\n') + '\r\n')
        }
        queueRef.current = []
        setQueuedCount(0)
        termRef.current?.scrollToBottom()
      }
      return next
    })
  }, [])

  const clearTerminal = useCallback(() => {
    termRef.current?.clear()
    queueRef.current = []
    setQueuedCount(0)
  }, [])

  const scrollToBottom = useCallback(() => {
    termRef.current?.scrollToBottom()
  }, [])

  // One effect owns the entire xterm + WS lifecycle, keyed on the runner
  // we're tailing. The WS handlers read from `handleIncomingRef.current`
  // (not a captured closure) so render-driven state changes don't tear
  // the WS down. The effect's only deps are `runnerId` and `tail` —
  // anything else would cause unwanted teardowns.
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

    // Initial fit may run before layout settles. ResizeObserver fires
    // after layout and on any subsequent container resize (e.g. tab
    // open/close), which the plain window-resize listener misses.
    const safeFit = () => {
      try {
        fit.fit()
      } catch {
        // ignore — happens during teardown if dimensions are zero
      }
    }
    safeFit()
    const ro =
      typeof ResizeObserver !== 'undefined' ? new ResizeObserver(safeFit) : null
    if (ro && containerRef.current) ro.observe(containerRef.current)
    window.addEventListener('resize', safeFit)

    let cancelled = false
    hasRetriedRef.current = false
    setStatus('connecting')

    const connect = (isRetry: boolean) => {
      if (cancelled) return
      setStatus(isRetry ? 'reconnecting' : 'connecting')

      const ws = new WebSocket(runnerLogsUrl(runnerId, tail))
      wsRef.current = ws

      ws.onopen = () => {
        if (cancelled) return
        setStatus('open')
        // Reset the retry budget so a long-lived connection that
        // eventually drops gets one more shot. Without this, every
        // future drop would be the second-and-final attempt.
        hasRetriedRef.current = false
      }
      ws.onmessage = (e) => {
        if (typeof e.data !== 'string') return
        try {
          const msg = JSON.parse(e.data) as { t?: string; line?: string }
          if (typeof msg.line === 'string') {
            handleIncomingRef.current(msg.line)
          }
        } catch {
          // Backend always emits JSON. Non-JSON is a contract break and
          // dumping it raw into the terminal as a "fallback" risks
          // visually corrupting the log view, so just drop with a console
          // warning.
          // eslint-disable-next-line no-console
          console.warn('[RunnerLogs] received non-JSON WS message; dropping')
        }
      }
      ws.onerror = () => {
        if (cancelled) return
        setStatus('error')
      }
      ws.onclose = () => {
        if (cancelled) return
        // Clear any in-flight reconnect timer before scheduling a new
        // one — without this a fast flap leaks setTimeouts.
        if (reconnectTimerRef.current) {
          clearTimeout(reconnectTimerRef.current)
          reconnectTimerRef.current = null
        }
        if (!hasRetriedRef.current) {
          hasRetriedRef.current = true
          setStatus('reconnecting')
          reconnectTimerRef.current = setTimeout(() => connect(true), reconnectDelayMs)
        } else {
          setStatus('closed')
        }
      }
    }

    connect(false)

    return () => {
      cancelled = true
      if (ro) ro.disconnect()
      window.removeEventListener('resize', safeFit)
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
      wsRef.current?.close()
      wsRef.current = null
      term.dispose()
      termRef.current = null
      fitRef.current = null
      // Reset paused-related state so a re-mount with a different
      // runner_id doesn't display a stale "N lines queued" hint.
      queueRef.current = []
      setQueuedCount(0)
      setPaused(false)
      pausedRef.current = false
      hasRetriedRef.current = false
    }
  }, [runnerId, tail])

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
        {showOpenInNewTab && (
          <Tooltip title="Open in new tab">
            <IconButton
              size="small"
              component="a"
              href={`/admin/runner-logs/${encodeURIComponent(runnerId)}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              <OpenInNewIcon fontSize="small" />
            </IconButton>
          </Tooltip>
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

// StatusChip renders the WS state as a labelled chip with a leading icon so
// the state is not communicated by colour alone (accessibility + colour-blind
// users). Icon + label + colour each carry the same signal.
const StatusChip: FC<{ status: ConnectionState }> = ({ status }) => {
  const view = (() => {
    switch (status) {
      case 'open':
        return {
          label: 'Live',
          color: 'success' as const,
          icon: <FiberManualRecordIcon sx={{ fontSize: 14 }} />,
        }
      case 'connecting':
        return {
          label: 'Connecting…',
          color: 'default' as const,
          icon: <HourglassEmptyIcon sx={{ fontSize: 14 }} />,
        }
      case 'reconnecting':
        return {
          label: 'Reconnecting…',
          color: 'warning' as const,
          icon: <SyncIcon sx={{ fontSize: 14 }} />,
        }
      case 'error':
        return {
          label: 'Error',
          color: 'error' as const,
          icon: <ErrorIcon sx={{ fontSize: 14 }} />,
        }
      case 'closed':
        return {
          label: 'Disconnected',
          color: 'error' as const,
          icon: <ErrorIcon sx={{ fontSize: 14 }} />,
        }
    }
  })()
  return <Chip size="small" label={view.label} color={view.color} icon={view.icon} />
}

export default RunnerLogs
