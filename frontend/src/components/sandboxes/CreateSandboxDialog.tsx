import { FC, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import {
  TypesCreateSandboxRequest,
  TypesSandbox,
  TypesSandboxRuntime,
} from '../../api/api'
import { useCreateSandbox } from '../../services/sandboxesService'

interface Props {
  open: boolean
  orgId: string
  onClose: () => void
  onCreated: (sandbox: TypesSandbox) => void
}

const RUNTIMES: { value: string; label: string; description: string }[] = [
  {
    value: TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop ?? 'ubuntu-desktop',
    label: 'Ubuntu Desktop',
    description: 'Full Ubuntu desktop, no agent autoboot. Stream the display, exec commands, transfer files.',
  },
]

// CreateSandboxDialog asks for a name, runtime, and optional TTL/env.
const CreateSandboxDialog: FC<Props> = ({ open, orgId, onClose, onCreated }) => {
  const [name, setName] = useState('')
  const [runtime, setRuntime] = useState<string>(RUNTIMES[0].value)
  const [ttlSeconds, setTtlSeconds] = useState<number>(3600)
  const [envText, setEnvText] = useState('')
  const [error, setError] = useState<string | undefined>()

  const createMutation = useCreateSandbox(orgId)

  const handleSubmit = async () => {
    setError(undefined)
    const env: Record<string, string> = {}
    for (const line of envText.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed) continue
      const eq = trimmed.indexOf('=')
      if (eq <= 0) {
        setError(`Invalid env line: "${trimmed}" (expected KEY=value)`)
        return
      }
      env[trimmed.slice(0, eq)] = trimmed.slice(eq + 1)
    }

    const payload: TypesCreateSandboxRequest = {
      name: name || undefined,
      runtime: runtime as TypesSandboxRuntime,
      timeout_seconds: ttlSeconds || undefined,
      env: Object.keys(env).length ? env : undefined,
    }
    try {
      const sandbox = await createMutation.mutateAsync(payload)
      onCreated(sandbox)
      // Reset for next open
      setName('')
      setEnvText('')
      setTtlSeconds(3600)
    } catch (e: any) {
      setError(e?.message || 'Failed to create sandbox')
    }
  }

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>New Sandbox</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField
            label="Name (optional)"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-sandbox"
            fullWidth
          />
          <TextField
            label="Runtime"
            select
            value={runtime}
            onChange={(e) => setRuntime(e.target.value)}
            fullWidth
            helperText={RUNTIMES.find((r) => r.value === runtime)?.description}
          >
            {RUNTIMES.map((r) => (
              <MenuItem key={r.value} value={r.value}>
                {r.label}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label="TTL (seconds)"
            type="text"
            value={ttlSeconds}
            onChange={(e) => {
              const n = parseInt(e.target.value, 10)
              setTtlSeconds(Number.isFinite(n) ? n : 0)
            }}
            helperText="Sandbox is automatically deleted after this many seconds. Default 1h."
            fullWidth
          />
          <TextField
            label="Environment variables"
            value={envText}
            onChange={(e) => setEnvText(e.target.value)}
            multiline
            minRows={3}
            fullWidth
            helperText="One KEY=value per line."
          />
          {error && <Typography color="error">{error}</Typography>}
          <Typography variant="caption" color="text.secondary">
            Resources are pinned at 1 vCPU / 2GB RAM in v1. The sandbox is ephemeral —
            nothing is persisted after deletion.
          </Typography>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={createMutation.isPending}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating…' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateSandboxDialog
