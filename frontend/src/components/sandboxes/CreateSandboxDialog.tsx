import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import {
  TypesCreateSandboxRequest,
  TypesSandbox,
  TypesSandboxRuntime,
} from '../../api/api'
import { useCreateSandbox } from '../../services/sandboxesService'
import { useListOrgApiKeys } from '../../services/orgApiKeyService'
import SandboxApiExamples from './SandboxApiExamples'

interface Props {
  open: boolean
  orgId: string
  onClose: () => void
  onCreated: (sandbox: TypesSandbox) => void
}

// Headless is listed first so it's the default — it's small, fast, has no GUI
// dependencies, and supports the full exec/files/terminal API. Pick the desktop
// runtime only if you need the streaming display.
const RUNTIMES: { value: string; label: string; description: string }[] = [
  {
    value: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu ?? 'headless-ubuntu',
    label: 'Headless Ubuntu',
    description: 'Plain ubuntu:22.04 running sleep infinity. No GUI — just exec commands and read/write files.',
  },
  {
    value: TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop ?? 'ubuntu-desktop',
    label: 'Ubuntu Desktop',
    description: 'Full Ubuntu desktop, no agent autoboot. Stream the display, exec commands, transfer files.',
  },
]

// PERSISTENT_WORKSPACE_PATH is the in-container directory that the API
// bind-mounts to a sandbox-host directory when persistent=true. Anything written
// outside this path is in the container's ephemeral overlay and goes away when
// the sandbox is deleted or the container is recreated.
const PERSISTENT_WORKSPACE_PATH = '/home/retro/work'

const RESOURCE_PRESETS = [
  { value: 'small', label: '1 CPU / 2GB RAM', vcpus: 1, memoryMB: 2048 },
  { value: 'medium', label: '4 CPU / 8GB RAM', vcpus: 4, memoryMB: 8192 },
  { value: 'large', label: '8 CPU / 16GB RAM', vcpus: 8, memoryMB: 16384 },
]

// CreateSandboxDialog asks for a name, runtime, and optional TTL/env.
const CreateSandboxDialog: FC<Props> = ({ open, orgId, onClose, onCreated }) => {
  const [name, setName] = useState('')
  const [runtime, setRuntime] = useState<string>(RUNTIMES[0].value)
  const [resourcePreset, setResourcePreset] = useState<string>(RESOURCE_PRESETS[0].value)
  const [autoExpire, setAutoExpire] = useState<boolean>(true)
  const [ttlSeconds, setTtlSeconds] = useState<number>(3600)
  const [envText, setEnvText] = useState('')
  const [persistent, setPersistent] = useState<boolean>(false)
  const [error, setError] = useState<string | undefined>()

  const createMutation = useCreateSandbox(orgId)
  // Fetch org API keys lazily — only when the dialog is open. The first one is
  // surfaced in the example snippets so the reader can copy & paste a working
  // export without bouncing through settings.
  const { data: orgApiKeys } = useListOrgApiKeys(orgId, open)
  const orgApiKey = orgApiKeys && orgApiKeys.length > 0 ? orgApiKeys[0].key : undefined

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

    // Backend convention: timeout_seconds < 0 means "never expire".
    const timeoutSeconds = autoExpire ? (ttlSeconds || undefined) : -1
    const resources = RESOURCE_PRESETS.find((preset) => preset.value === resourcePreset) ?? RESOURCE_PRESETS[0]

    const payload: TypesCreateSandboxRequest = {
      name: name || undefined,
      runtime: runtime as TypesSandboxRuntime,
      timeout_seconds: timeoutSeconds,
      vcpus: resources.vcpus,
      memory_mb: resources.memoryMB,
      env: Object.keys(env).length ? env : undefined,
      persistent,
    }
    try {
      const sandbox = await createMutation.mutateAsync(payload)
      onCreated(sandbox)
      // Reset for next open
      setName('')
      setEnvText('')
      setResourcePreset(RESOURCE_PRESETS[0].value)
      setAutoExpire(true)
      setTtlSeconds(3600)
      setPersistent(false)
    } catch (e: any) {
      setError(e?.message || 'Failed to create sandbox')
    }
  }

  const resourceForExamples = RESOURCE_PRESETS.find((p) => p.value === resourcePreset) ?? RESOURCE_PRESETS[0]

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xl">
      <DialogTitle>New Sandbox</DialogTitle>
      <DialogContent
        dividers
        sx={{
          display: 'flex',
          gap: 0,
          p: 0,
          // Cap dialog body height so each column scrolls independently
          // instead of the whole dialog growing past the viewport.
          height: '75vh',
        }}
      >
        <Stack spacing={2} sx={{ flex: '0 0 420px', minWidth: 0, p: 3, overflowY: 'auto' }}>
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
            label="Resources"
            select
            value={resourcePreset}
            onChange={(e) => setResourcePreset(e.target.value)}
            fullWidth
            helperText="Sandbox billing is charged per core-second, so larger sizes consume credits faster."
          >
            {RESOURCE_PRESETS.map((preset) => (
              <MenuItem key={preset.value} value={preset.value}>
                {preset.label}
              </MenuItem>
            ))}
          </TextField>
          <Box>
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControlLabel
                control={
                  <Switch
                    checked={autoExpire}
                    onChange={(e) => setAutoExpire(e.target.checked)}
                  />
                }
                label="Auto-expire"
                sx={{ whiteSpace: 'nowrap' }}
              />
              <TextField
                label="TTL (seconds)"
                type="text"
                value={ttlSeconds}
                onChange={(e) => {
                  const n = parseInt(e.target.value, 10)
                  setTtlSeconds(Number.isFinite(n) ? n : 0)
                }}
                disabled={!autoExpire}
                fullWidth
              />
            </Stack>
            <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
              {autoExpire
                ? 'Sandbox is automatically deleted after this many seconds. Default 1h.'
                : 'Auto-expire is off — this sandbox will run until you delete it manually.'}
            </Typography>
          </Box>
          <Box>
            <FormControlLabel
              control={
                <Switch
                  checked={persistent}
                  onChange={(e) => setPersistent(e.target.checked)}
                />
              }
              label="Persistent workspace"
            />
            <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
              {persistent ? (
                <>
                  Data written to <code>{PERSISTENT_WORKSPACE_PATH}</code> is bind-mounted to the host and
                  survives container restarts and crashes. Everything else (system packages, /tmp, /root)
                  is ephemeral and resets when the container is recreated.
                </>
              ) : (
                <>
                  No persistent storage. Anything you write — including to <code>{PERSISTENT_WORKSPACE_PATH}</code> —
                  is lost when the sandbox is deleted or its container is recreated.
                </>
              )}
            </Typography>
          </Box>
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
        </Stack>
        <Box
          sx={{
            flex: '1 1 0',
            minWidth: 0,
            borderLeft: '1px solid',
            borderColor: 'divider',
            p: 3,
            display: { xs: 'none', md: 'flex' },
            flexDirection: 'column',
            // Match parent height; SandboxApiExamples handles its own scroll.
            height: '100%',
            overflow: 'hidden',
          }}
        >
          <SandboxApiExamples
            orgId={orgId}
            name={name}
            runtime={runtime}
            vcpus={resourceForExamples.vcpus}
            memoryMb={resourceForExamples.memoryMB}
            timeoutSeconds={autoExpire ? ttlSeconds : -1}
            persistent={persistent}
            apiKey={orgApiKey}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={createMutation.isPending}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="secondary"
          disabled={createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating…' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateSandboxDialog
