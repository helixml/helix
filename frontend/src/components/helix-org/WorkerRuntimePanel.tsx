// WorkerRuntimePanel renders the helix-org worker.* runtime config
// (runtime / credentials / provider / model) as first-class dropdowns.
// It lives on the AI Providers page so an operator configures providers
// and then picks which one their Workers use in one place. Reads/writes
// the same config registry the server validates against; org context is
// resolved from router.params.org_id by the underlying hooks.
//
// The provider + model dropdowns pull their options from Helix's existing
// /providers + /v1/models endpoints, so adding a provider is reflected
// here without a redeploy.

import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import FormHelperText from '@mui/material/FormHelperText'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Paper from '@mui/material/Paper'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import SaveIcon from '@mui/icons-material/Save'

import LoadingSpinner from '../widgets/LoadingSpinner'
import useSnackbar from '../../hooks/useSnackbar'
import {
  SettingsSpecDTO,
  useHelixModelsForProvider,
  useHelixOrgSettings,
  useHelixProviders,
  useSetHelixOrgSetting,
} from '../../services/helixOrgService'

// Allowed values for the two dropdown-only knobs. The server validates
// against the same set in api/pkg/server/helix_org.go::resolveWorkerAgentConfig.
const RUNTIME_OPTIONS = [
  { value: 'claude_code', label: 'claude_code', help: 'Anthropic Claude Code CLI inside the desktop. Authenticates via subscription OAuth or via Helix-routed API key.' },
  { value: 'zed_agent', label: 'zed_agent', help: 'Native Zed agent — always routed through a Helix-managed provider (forces credentials=api_key).' },
]

const CREDENTIALS_OPTIONS = [
  { value: 'subscription', label: 'subscription', help: 'Each operator authenticates with their own Claude OAuth subscription (only meaningful with runtime=claude_code).' },
  { value: 'api_key', label: 'api_key', help: 'Inference routed through one of Helix\'s configured providers — pick provider + model below.' },
]

// JSON-encode the value the dropdown produced so it satisfies the
// registry's string-spec contract.
const encodeStringValue = (raw: string): string => JSON.stringify(raw)

// Read the redacted/persisted JSON value back into a plain string for the
// dropdowns. Falls back to empty when parsing fails (e.g. masked value).
const decodeStringValue = (v: string): string => {
  if (!v) return ''
  try {
    const parsed = JSON.parse(v)
    return typeof parsed === 'string' ? parsed : ''
  } catch {
    return v
  }
}

const WorkerRuntimePanel: FC = () => {
  const { data, isLoading } = useHelixOrgSettings()
  const { data: providers } = useHelixProviders()

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])

  const initialRuntime = decodeStringValue(specByKey.get('worker.runtime')?.value ?? '') || 'claude_code'
  const initialCreds = decodeStringValue(specByKey.get('worker.credentials')?.value ?? '') || 'subscription'
  const initialProvider = decodeStringValue(specByKey.get('worker.provider')?.value ?? '')
  const initialModel = decodeStringValue(specByKey.get('worker.model')?.value ?? '')

  const [runtime, setRuntime] = useState<string>(initialRuntime)
  const [credentials, setCredentials] = useState<string>(initialCreds)
  const [provider, setProvider] = useState<string>(initialProvider)
  const [model, setModel] = useState<string>(initialModel)

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setRuntime(initialRuntime)
    setCredentials(initialCreds)
    setProvider(initialProvider)
    setModel(initialModel)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const { data: models } = useHelixModelsForProvider(provider, { enabled: credentials === 'api_key' })

  // claude_code + subscription means provider+model are unused (the CLI
  // handles OAuth itself), so we visually disable those rows.
  const apiKeyMode = credentials === 'api_key'

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Stack spacing={2.5}>
        {isLoading ? (
          <LoadingSpinner />
        ) : (
          <>
            <RuntimeRow value={runtime} onChange={setRuntime} />
            <CredentialsRow
              value={credentials}
              onChange={(v) => {
                setCredentials(v)
                // Switching to subscription wipes provider+model because
                // they're meaningless in OAuth mode.
                if (v === 'subscription' && runtime === 'claude_code') {
                  setProvider('')
                  setModel('')
                }
              }}
              runtime={runtime}
            />
            <ProviderRow
              value={provider}
              onChange={(v) => { setProvider(v); setModel('') }}
              providers={providers ?? []}
              disabled={!apiKeyMode}
            />
            <ModelRow
              value={model}
              onChange={setModel}
              models={models ?? []}
              disabled={!apiKeyMode || !provider}
              provider={provider}
            />
          </>
        )}
      </Stack>
    </Paper>
  )
}

// RuntimeRow is the worker.runtime dropdown.
const RuntimeRow: FC<{ value: string; onChange: (v: string) => void }> = ({ value, onChange }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.runtime', value: encodeStringValue(value) })
      snackbar.success('Runtime saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const helpFor = RUNTIME_OPTIONS.find((o) => o.value === value)?.help
  return (
    <FormControl fullWidth size="small">
      <InputLabel id="runtime-label">Runtime</InputLabel>
      <Select
        labelId="runtime-label"
        value={value}
        label="Runtime"
        onChange={(e) => onChange(e.target.value)}
      >
        {RUNTIME_OPTIONS.map((o) => (
          <MenuItem key={o.value} value={o.value} sx={{ fontFamily: 'monospace' }}>
            {o.label}
          </MenuItem>
        ))}
      </Select>
      <FormHelperText>{helpFor}</FormHelperText>
      <Box sx={{ mt: 1 }}>
        <Button size="small" variant="contained" color="secondary" startIcon={<SaveIcon />} onClick={handleSave} disabled={setMut.isPending}>
          {setMut.isPending ? 'Saving…' : 'Save runtime'}
        </Button>
      </Box>
    </FormControl>
  )
}

// CredentialsRow is the worker.credentials dropdown.
const CredentialsRow: FC<{ value: string; onChange: (v: string) => void; runtime: string }> = ({ value, onChange, runtime }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.credentials', value: encodeStringValue(value) })
      snackbar.success('Credentials saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const helpFor = CREDENTIALS_OPTIONS.find((o) => o.value === value)?.help
  // The backend coerces non-claude_code runtimes to api_key, but surface
  // that so the page doesn't look broken on zed_agent + subscription.
  const forcedToApiKey = runtime !== 'claude_code'
  return (
    <FormControl fullWidth size="small">
      <InputLabel id="creds-label">Credentials</InputLabel>
      <Select
        labelId="creds-label"
        value={forcedToApiKey ? 'api_key' : value}
        label="Credentials"
        onChange={(e) => onChange(e.target.value)}
        disabled={forcedToApiKey}
      >
        {CREDENTIALS_OPTIONS.map((o) => (
          <MenuItem key={o.value} value={o.value} sx={{ fontFamily: 'monospace' }}>
            {o.label}
          </MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {forcedToApiKey ? `runtime=${runtime} forces api_key — only claude_code supports subscription.` : helpFor}
      </FormHelperText>
      <Box sx={{ mt: 1 }}>
        <Button size="small" variant="contained" color="secondary" startIcon={<SaveIcon />} onClick={handleSave} disabled={setMut.isPending || forcedToApiKey}>
          {setMut.isPending ? 'Saving…' : 'Save credentials'}
        </Button>
      </Box>
    </FormControl>
  )
}

// ProviderRow lists every provider configured on this Helix instance.
const ProviderRow: FC<{
  value: string
  onChange: (v: string) => void
  providers: string[]
  disabled: boolean
}> = ({ value, onChange, providers, disabled }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.provider', value: encodeStringValue(value) })
      snackbar.success('Provider saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  return (
    <FormControl fullWidth size="small" disabled={disabled}>
      <InputLabel id="provider-label">Provider</InputLabel>
      <Select
        labelId="provider-label"
        value={value}
        label="Provider"
        onChange={(e) => onChange(e.target.value)}
      >
        <MenuItem value=""><em>none</em></MenuItem>
        {providers.map((p) => (
          <MenuItem key={p} value={p} sx={{ fontFamily: 'monospace' }}>{p}</MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {disabled
          ? 'Pick credentials=api_key above to enable.'
          : 'One of the providers configured above on this Helix instance.'}
      </FormHelperText>
      <Box sx={{ mt: 1 }}>
        <Button size="small" variant="contained" color="secondary" startIcon={<SaveIcon />} onClick={handleSave} disabled={setMut.isPending || disabled}>
          {setMut.isPending ? 'Saving…' : 'Save provider'}
        </Button>
      </Box>
    </FormControl>
  )
}

// ModelRow lists models the picked provider exposes.
const ModelRow: FC<{
  value: string
  onChange: (v: string) => void
  models: { id: string; name?: string; description?: string }[]
  disabled: boolean
  provider: string
}> = ({ value, onChange, models, disabled, provider }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.model', value: encodeStringValue(value) })
      snackbar.success('Model saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const selected = models.find((m) => m.id === value)
  return (
    <FormControl fullWidth size="small" disabled={disabled}>
      <InputLabel id="model-label">Model</InputLabel>
      <Select
        labelId="model-label"
        value={value}
        label="Model"
        onChange={(e) => onChange(e.target.value)}
      >
        <MenuItem value=""><em>none</em></MenuItem>
        {models.map((m) => (
          <MenuItem key={m.id} value={m.id} sx={{ fontFamily: 'monospace' }}>
            {m.id}{m.name ? ` — ${m.name}` : ''}
          </MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {disabled
          ? (provider ? 'Pick credentials=api_key above to enable.' : 'Pick a provider first.')
          : (selected?.description?.slice(0, 200) ?? `Models exposed by ${provider}.`)
        }
      </FormHelperText>
      <Box sx={{ mt: 1 }}>
        <Button size="small" variant="contained" color="secondary" startIcon={<SaveIcon />} onClick={handleSave} disabled={setMut.isPending || disabled}>
          {setMut.isPending ? 'Saving…' : 'Save model'}
        </Button>
      </Box>
    </FormControl>
  )
}

export default WorkerRuntimePanel
