import { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import StopIcon from '@mui/icons-material/Stop'

import SimpleTable from '../widgets/SimpleTable'

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
    try {
      const resp = await runMutation.mutateAsync({
        cmd: trimmed,
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

  const tableFields = useMemo(() => [
    { name: 'command', title: 'Command' },
    { name: 'status', title: 'Status' },
    { name: 'exit', title: 'Exit' },
    { name: 'started', title: 'Started' },
  ], [])

  const tableData = useMemo(() => (commands?.commands ?? []).map((c) => ({
    id: c.id,
    _data: c,
    command: (
      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: 13 }}>
        {[c.cmd, ...(c.args ?? [])].join(' ')}
      </Typography>
    ),
    status: <Chip size="small" label={c.status} color={statusColor[c.status ?? 'default'] ?? 'default'} />,
    exit: c.exit_code ?? '-',
    started: c.started_at ? new Date(c.started_at).toLocaleTimeString() : '-',
  })), [commands])

  const getActions = useCallback((row: Record<string, any>) => {
    const c = row._data
    if (c?.status !== 'running') return <Box />
    return (
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
    )
  }, [handleKill])

  return (
    <Stack spacing={2}>
      <Box>
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
            color="secondary"
            startIcon={<PlayArrowIcon />}
            disabled={!running || runMutation.isPending}
            onClick={handleRun}
          >
            Run
          </Button>
        </Stack>
        {!running && (
          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
            Sandbox is not running yet.
          </Typography>
        )}
      </Box>

      <Box display="flex" gap={2}>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <SimpleTable
            authenticated={true}
            fields={tableFields}
            data={tableData}
            getActions={getActions}
            onRowClick={(row) => setSelectedCmdId(row.id)}
          />
        </Box>

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
