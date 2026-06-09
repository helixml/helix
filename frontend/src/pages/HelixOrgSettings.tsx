// HelixOrgSettings is the configuration surface for the helix-org
// alpha. The "headline" rows — worker.runtime / credentials / provider
// / model — are rendered as first-class dropdowns so an operator can
// switch between claude_code subscription mode (the SaaS default, each
// operator OAuths their own Claude account) and api_key mode (inference
// routed through one of Helix's configured providers) without having to
// know the wire form. Everything else falls back to a generic
// text-input row driven by the same config registry the server
// validates against.
//
// The provider + model dropdowns pull their options from Helix's
// existing /providers + /v1/models endpoints, so changing or adding a
// provider in the Helix admin surface is automatically reflected here
// without a redeploy.

import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import FormControl from '@mui/material/FormControl'
import FormHelperText from '@mui/material/FormHelperText'
import InputLabel from '@mui/material/InputLabel'
import Link from '@mui/material/Link'
import MenuItem from '@mui/material/MenuItem'
import Paper from '@mui/material/Paper'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import SaveIcon from '@mui/icons-material/Save'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import GitHubAppPanel from '../components/helix-org/GitHubAppPanel'
import {
  SettingsSpecDTO,
  useDeleteHelixOrgSetting,
  useHelixModelsForProvider,
  useHelixOrgSettings,
  useHelixProviders,
  useSetHelixOrgSetting,
} from '../services/helixOrgService'

// Keys we render as first-class controls. Anything not in here falls
// through to the generic TextField row below.
const FIRST_CLASS_KEYS = new Set<string>([
  'worker.runtime',
  'worker.credentials',
  'worker.provider',
  'worker.model',
])

// Keys we deliberately hide from the settings surface. transport.github
// is auto-managed now: the Helix GitHub App provisions the webhook
// secret (and the access token comes from the App installation), so
// there's nothing for an operator to paste here. The key still exists
// in the registry — it's just self-configured, not operator-facing.
const HIDDEN_KEYS = new Set<string>([
  'transport.github',
])

// Allowed values for the two dropdown-only knobs. The server
// validates against the same set in
// api/pkg/server/helix_org.go::resolveWorkerAgentConfig.
const RUNTIME_OPTIONS = [
  { value: 'claude_code', label: 'claude_code', help: 'Anthropic Claude Code CLI inside the desktop. Authenticates via subscription OAuth or via Helix-routed API key.' },
  { value: 'zed_agent', label: 'zed_agent', help: 'Native Zed agent — always routed through a Helix-managed provider (forces credentials=api_key).' },
]

const CREDENTIALS_OPTIONS = [
  { value: 'subscription', label: 'subscription', help: 'Each operator authenticates with their own Claude OAuth subscription (only meaningful with runtime=claude_code).' },
  { value: 'api_key', label: 'api_key', help: 'Inference routed through one of Helix\'s configured providers — pick provider + model below.' },
]

// JSON-encode the value the dropdown produced so it satisfies the
// registry's string-spec contract. Empty string → empty value (clears).
const encodeStringValue = (raw: string): string => JSON.stringify(raw)

// Read the redacted/persisted JSON value back into a plain string for
// the dropdowns. Falls back to empty when parsing fails (e.g. the
// stored value is masked).
const decodeStringValue = (v: string): string => {
  if (!v) return ''
  try {
    const parsed = JSON.parse(v)
    return typeof parsed === 'string' ? parsed : ''
  } catch {
    return v
  }
}

const HelixOrgSettings: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const orgSlug = router.params.org_id as string | undefined

  const { data, isLoading } = useHelixOrgSettings()
  const { data: providers } = useHelixProviders()

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])

  // Initial values from the loaded settings — decoded from the
  // registry's JSON wire form into plain strings the dropdowns can
  // bind to.
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

  // claude_code + subscription means provider+model are unused (the
  // CLI handles OAuth itself), so we visually disable those rows.
  const apiKeyMode = credentials === 'api_key'

  return (
    <Page
      breadcrumbTitle="Settings"
      orgBreadcrumbs={true}
      orgBreadcrumbRouteName="helix_org_chart"
      orgBreadcrumbRouteParams={{ org_id: orgSlug ?? '' }}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="md" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={3}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Settings</Typography>
            <Typography variant="body2" color="text.secondary">
              Configures how this org's Workers run. Changes take effect on the next worker
              activation — no API restart needed.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : (
            <>
              <Paper variant="outlined" sx={{ p: 3 }}>
                <Stack spacing={2.5}>
                  <Typography variant="subtitle1">Worker runtime</Typography>
                  <RuntimeRow value={runtime} onChange={setRuntime} />
                  <CredentialsRow
                    value={credentials}
                    onChange={(v) => {
                      setCredentials(v)
                      // Switching to subscription wipes provider+model
                      // because they're meaningless in OAuth mode.
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
                    orgSlug={orgSlug}
                  />
                  <ModelRow
                    value={model}
                    onChange={setModel}
                    models={models ?? []}
                    disabled={!apiKeyMode || !provider}
                    provider={provider}
                  />
                </Stack>
              </Paper>

              <GitHubAppPanel />

              {/* Generic spec rows — everything not first-class or hidden */}
              {(data?.specs ?? [])
                .filter((s) => !FIRST_CLASS_KEYS.has(s.key) && !HIDDEN_KEYS.has(s.key))
                .map((s) => <GenericSettingRow key={s.key} spec={s} />)}
            </>
          )}
        </Stack>
      </Container>
    </Page>
  )
}

// RuntimeRow is the worker.runtime dropdown.
const RuntimeRow: FC<{ value: string; onChange: (v: string) => void }> = ({ value, onChange }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.runtime', value: encodeStringValue(value) })
      snackbar.success('worker.runtime saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const helpFor = RUNTIME_OPTIONS.find((o) => o.value === value)?.help
  return (
    <FormControl fullWidth size="small">
      <InputLabel id="runtime-label">worker.runtime</InputLabel>
      <Select
        labelId="runtime-label"
        value={value}
        label="worker.runtime"
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
      snackbar.success('worker.credentials saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const helpFor = CREDENTIALS_OPTIONS.find((o) => o.value === value)?.help
  // The backend coerces non-claude_code runtimes to api_key, but
  // surface that fact to the operator so the page doesn't look
  // broken when they pick zed_agent + subscription.
  const forcedToApiKey = runtime !== 'claude_code'
  return (
    <FormControl fullWidth size="small">
      <InputLabel id="creds-label">worker.credentials</InputLabel>
      <Select
        labelId="creds-label"
        value={forcedToApiKey ? 'api_key' : value}
        label="worker.credentials"
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
  orgSlug: string | undefined
}> = ({ value, onChange, providers, disabled, orgSlug }) => {
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()
  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: 'worker.provider', value: encodeStringValue(value) })
      snackbar.success('worker.provider saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  return (
    <FormControl fullWidth size="small" disabled={disabled}>
      <InputLabel id="provider-label">worker.provider</InputLabel>
      <Select
        labelId="provider-label"
        value={value}
        label="worker.provider"
        onChange={(e) => onChange(e.target.value)}
      >
        <MenuItem value=""><em>none</em></MenuItem>
        {providers.map((p) => (
          <MenuItem key={p} value={p} sx={{ fontFamily: 'monospace' }}>{p}</MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {disabled ? (
          'Pick credentials=api_key above to enable.'
        ) : (
          <span>
            One of the providers configured on this Helix instance. Missing one?{' '}
            <Link
              href={orgSlug ? `/orgs/${orgSlug}/providers` : '#'}
              underline="hover"
              sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25 }}
            >
              Manage providers <OpenInNewIcon sx={{ fontSize: 12 }} />
            </Link>
          </span>
        )}
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
      snackbar.success('worker.model saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const selected = models.find((m) => m.id === value)
  return (
    <FormControl fullWidth size="small" disabled={disabled}>
      <InputLabel id="model-label">worker.model</InputLabel>
      <Select
        labelId="model-label"
        value={value}
        label="worker.model"
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

// GenericSettingRow renders any registry spec we don't have a
// dedicated control for (helix.url, helix.api_key, worker.specs_mandate,
// …) and isn't in HIDDEN_KEYS. Plain text input. Secrets are shown
// redacted and must be re-entered to update.
const GenericSettingRow: FC<{ spec: SettingsSpecDTO }> = ({ spec }) => {
  const setMut = useSetHelixOrgSetting()
  const delMut = useDeleteHelixOrgSetting()
  const snackbar = useSnackbar()
  const [value, setValue] = useState('')
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    setValue(spec.value ?? '')
    setDirty(false)
  }, [spec.value, spec.configured])

  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: spec.key, value })
      snackbar.success(`${spec.key} saved`)
      setDirty(false)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }
  const handleClear = async () => {
    try {
      await delMut.mutateAsync(spec.key)
      snackbar.success(`${spec.key} cleared`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'clear failed')
    }
  }
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>{spec.key}</Typography>
          <Chip size="small" label={spec.type} sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }} />
          {spec.required && <Chip size="small" color="warning" label="required" />}
          {spec.configured && <Chip size="small" color="success" label="configured" />}
        </Stack>
        {spec.description && (
          <Typography variant="caption" color="text.secondary">{spec.description}</Typography>
        )}
        <TextField
          fullWidth
          size="small"
          value={value}
          onChange={(e) => { setValue(e.target.value); setDirty(true) }}
          placeholder={spec.type === 'string' ? '"plain string (no quotes needed in UI)"' : 'raw JSON per spec type'}
          multiline={spec.type !== 'string'}
          minRows={spec.type !== 'string' ? 3 : undefined}
        />
        <Stack direction="row" spacing={1}>
          <Button size="small" variant="contained" color="secondary" startIcon={<SaveIcon />} onClick={handleSave} disabled={!dirty || setMut.isPending}>
            {setMut.isPending ? 'Saving…' : 'Save'}
          </Button>
          {spec.configured && (
            <Button size="small" color="error" onClick={handleClear} disabled={delMut.isPending}>
              Clear
            </Button>
          )}
        </Stack>
      </Stack>
    </Paper>
  )
}

export default HelixOrgSettings
