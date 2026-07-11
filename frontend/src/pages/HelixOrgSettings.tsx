// HelixOrgSettings is the configuration surface for the helix-org alpha.
// The worker.* runtime config (runtime / credentials / provider / model)
// now lives on the AI Providers page (WorkerRuntimePanel) so it sits next
// to the providers it references; everything else here falls back to a
// generic text-input row driven by the same config registry the server
// validates against.

import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import SaveIcon from '@mui/icons-material/Save'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useSnackbar from '../hooks/useSnackbar'
import GitHubAppPanel from '../components/helix-org/GitHubAppPanel'
import SlackIntegrationsPanel from '../components/helix-org/SlackIntegrationsPanel'
import {
  SettingsSpecDTO,
  useDeleteHelixOrgSetting,
  useHelixOrgSettings,
  useSetHelixOrgSetting,
} from '../services/helixOrgService'

// Registry keys we do NOT render as generic rows here:
//  - worker.* runtime config lives on the Providers page (WorkerRuntimePanel).
//  - transport.github is auto-managed by the Helix GitHub App (it provisions
//    the webhook secret and the token comes from the App installation), so
//    there's nothing for an operator to paste.
const EXCLUDED_KEYS = new Set<string>([
  'worker.runtime',
  'worker.credentials',
  'worker.provider',
  'worker.model',
  'transport.github',
])

const HelixOrgSettings: FC = () => {

  const { data, isLoading } = useHelixOrgSettings()

  return (
    <HelixOrgShell showChat={false}>
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="md" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={3}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Settings</Typography>
            <Typography variant="body2" color="text.secondary">
              Configures how this org's Bots run. Changes take effect on the next bot
              activation — no API restart needed.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : (
            <>
              <GitHubAppPanel />

              <SlackIntegrationsPanel />

              {/* Generic spec rows — everything not excluded above */}
              {(data?.specs ?? [])
                .filter((s) => !EXCLUDED_KEYS.has(s.key))
                .map((s) => <GenericSettingRow key={s.key} spec={s} />)}
            </>
          )}
        </Stack>
      </Container>
      </Box>
    </HelixOrgShell>
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
