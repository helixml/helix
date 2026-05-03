import { FC, useState, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import StopIcon from '@mui/icons-material/Stop'

import {
  useSandboxCommands,
  useRunSandboxCommand,
  useKillSandboxCommand,
} from '../../services/sandboxesService'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
}

const statusColor: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'error'> = {
  pending: 'default',
  running: 'primary',
  finished: 'success',
  failed: 'error',
  killed: 'warning',
}

const SandboxCommandsTab: FC<Props> = ({ orgId, sandboxId, running }) => {
  const [cmd, setCmd] = useState('')
  const [selectedCmdId, setSelectedCmdId] = useState<string | undefined>()
  const [logs, setLogs] = useState<string>('')
  const evtRef = useRef<EventSource | undefined>()

  const { data: commands } = useSandboxCommands(orgId, sandboxId, { enabled: !!sandboxId })
  const runMutation = useRunSandboxCommand(orgId, sandboxId)
  const killMutation = useKillSandboxCommand(orgId, sandboxId)

  // Stream logs for the selected command via SSE.
  useEffect(() => {
    evtRef.current?.close()
    setLogs('')
    if (!selectedCmdId) return
    const es = new EventSource(
      `/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands/${selectedCmdId}/logs?stream=both&follow=1`
    )
    evtRef.current = es
    const append = (e: MessageEvent) => {
      try {
        const text = JSON.parse(e.data) as string
        setLogs((prev) => prev + text)
      } catch {
        setLogs((prev) => prev + e.data)
      }
    }
    es.addEventListener('stdout', append)
    es.addEventListener('stderr', append)
    es.addEventListener('end', () => es.close())
    es.onerror = () => es.close()
    return () => es.close()
  }, [orgId, sandboxId, selectedCmdId])

  const handleRun = async () => {
    if (!cmd.trim()) return
    const trimmed = cmd.trim()
    const tokens = trimmed.match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) ?? [trimmed]
    const argv = tokens.map((t) => t.replace(/^['"]|['"]$/g, ''))
    const head = argv[0] ?? trimmed
    const args = argv.slice(1)
    try {
      const resp = await runMutation.mutateAsync({
        cmd: head,
        args: args.length ? args : undefined,
        detached: true,
        timeout_seconds: 0,
      })
      const newId = (resp as unknown as { id?: string })?.id
      if (newId) setSelectedCmdId(newId)
      setCmd('')
    } catch {
      // snackbar handled globally
    }
  }

  const handleKill = (cmdId: string) => killMutation.mutate({ cmdId })

  return (
    <Stack spacing={2}>
      <Paper sx={{ p: 2 }}>
        <Stack direction="row" spacing={1}>
          <TextField
            fullWidth
            placeholder="ls -la /home"
            value={cmd}
            onChange={(e) => setCmd(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleRun()
            }}
            disabled={!running}
            size="small"
          />
          <Button
            variant="contained"
            color="primary"
            startIcon={<PlayArrowIcon />}
            disabled={!running || runMutation.isPending}
            onClick={handleRun}
          >
            Run
          </Button>
        </Stack>
        {!running && (
          <Typography variant="caption" color="text.secondary">
            Sandbox is not running yet.
          </Typography>
        )}
      </Paper>

      <Box display="flex" gap={2}>
        <TableContainer component={Paper} sx={{ flex: 1 }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Command</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Exit</TableCell>
                <TableCell>Started</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {(commands?.commands ?? []).map((c) => (
                <TableRow
                  key={c.id}
                  hover
                  selected={c.id === selectedCmdId}
                  onClick={() => setSelectedCmdId(c.id)}
                  sx={{ cursor: 'pointer' }}
                >
                  <TableCell sx={{ fontFamily: 'monospace', fontSize: 12 }}>
                    {[c.cmd, ...(c.args ?? [])].join(' ')}
                  </TableCell>
                  <TableCell>
                    <Chip size="small" label={c.status} color={statusColor[c.status] ?? 'default'} />
                  </TableCell>
                  <TableCell>{c.exit_code ?? '-'}</TableCell>
                  <TableCell>{c.started_at ? new Date(c.started_at).toLocaleTimeString() : '-'}</TableCell>
                  <TableCell align="right">
                    {c.status === 'running' && (
                      <Tooltip title="Send SIGTERM">
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation()
                            handleKill(c.id)
                          }}
                        >
                          <StopIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                  </TableCell>
                </TableRow>
              ))}
              {(commands?.commands ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={5}>
                    <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                      No commands yet.
                    </Typography>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>

        <Paper sx={{ flex: 1, p: 2, minHeight: 320, maxHeight: 480, overflow: 'auto', bgcolor: '#0b0b0b' }}>
          <Typography variant="caption" color="text.secondary">
            {selectedCmdId ? `Logs for ${selectedCmdId}` : 'Select a command to view logs'}
          </Typography>
          <Box
            component="pre"
            sx={{
              fontFamily: 'monospace',
              fontSize: 12,
              whiteSpace: 'pre-wrap',
              color: '#e5e5e5',
              m: 0,
              mt: 1,
            }}
          >
            {logs}
          </Box>
        </Paper>
      </Box>
    </Stack>
  )
}

export default SandboxCommandsTab
