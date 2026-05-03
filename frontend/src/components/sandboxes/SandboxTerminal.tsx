import { FC, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

import { sandboxTerminalUrl } from '../../services/sandboxesService'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
}

// SandboxTerminal renders an xterm.js terminal connected to the sandbox via
// a websocket. Frame protocol:
//   Browser → server: binary stdin, text JSON for control (resize).
//   Server → browser: binary stdout/stderr (TTY-merged).
const SandboxTerminal: FC<Props> = ({ orgId, sandboxId, running }) => {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | undefined>()
  const fitRef = useRef<FitAddon | undefined>()
  const wsRef = useRef<WebSocket | undefined>()

  useEffect(() => {
    if (!running || !containerRef.current) return
    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      theme: { background: '#000000' },
      convertEol: true,
      cursorBlink: true,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    termRef.current = term
    fitRef.current = fit
    fit.fit()

    const ws = new WebSocket(sandboxTerminalUrl(orgId, sandboxId))
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
      term.focus()
    }
    ws.onmessage = (e) => {
      if (typeof e.data === 'string') {
        // Text frame — likely an error message in JSON form.
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

    const onData = (data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    }
    term.onData(onData)

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
  }, [orgId, sandboxId, running])

  if (!running) {
    return (
      <Paper sx={{ p: 4, textAlign: 'center' }}>
        <Typography variant="body2" color="text.secondary">
          Sandbox is not running yet — terminal will be available when status is "running".
        </Typography>
      </Paper>
    )
  }

  return (
    <Paper sx={{ p: 1, bgcolor: '#000', height: 480 }}>
      <Box ref={containerRef} sx={{ width: '100%', height: '100%' }} />
    </Paper>
  )
}

export default SandboxTerminal
