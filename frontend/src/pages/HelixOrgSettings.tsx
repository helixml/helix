import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import DeleteIcon from '@mui/icons-material/Delete'
import SaveIcon from '@mui/icons-material/Save'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useSnackbar from '../hooks/useSnackbar'
import {
  SettingsSpecDTO,
  useDeleteHelixOrgSetting,
  useHelixOrgSettings,
  useSetHelixOrgSetting,
} from '../services/helixOrgService'

// SettingRow is one editable spec entry. The backend stores values as
// raw JSON (string-typed specs expect a JSON-encoded string, etc.), so
// the input is intentionally a textarea with monospace styling: callers
// supply the wire form directly. Redacted values are display-only and
// must be re-entered to update.
const SettingRow: FC<{ spec: SettingsSpecDTO }> = ({ spec }) => {
  const snackbar = useSnackbar()
  const setMut = useSetHelixOrgSetting()
  const delMut = useDeleteHelixOrgSetting()

  const [value, setValue] = useState('')
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    setValue(spec.value ?? '')
    setDirty(false)
  }, [spec.value, spec.configured])

  const handleSave = async () => {
    try {
      await setMut.mutateAsync({ key: spec.key, value })
      setDirty(false)
      snackbar.success(`${spec.key} saved`)
    } catch (e) {
      snackbar.error(`Failed to save ${spec.key}`)
    }
  }

  const handleDelete = async () => {
    try {
      await delMut.mutateAsync(spec.key)
      snackbar.success(`${spec.key} cleared`)
    } catch (e) {
      snackbar.error(`Failed to clear ${spec.key}`)
    }
  }

  const isSecret = spec.type.toLowerCase().includes('secret')

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
        <Box>
          <Stack direction="row" alignItems="center" spacing={1}>
            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
              {spec.key}
            </Typography>
            <Chip
              size="small"
              label={spec.type}
              sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }}
            />
            {spec.required && (
              <Chip size="small" color="warning" label="required" sx={{ fontSize: '0.65rem' }} />
            )}
            {spec.configured ? (
              <Chip size="small" color="success" label="configured" sx={{ fontSize: '0.65rem' }} />
            ) : (
              <Chip size="small" label="not configured" sx={{ fontSize: '0.65rem' }} />
            )}
          </Stack>
          {spec.description && (
            <Typography variant="caption" color="text.secondary">
              {spec.description}
            </Typography>
          )}
        </Box>
        <Stack direction="row" spacing={1}>
          <Button
            size="small"
            variant="contained"
            color="secondary"
            startIcon={<SaveIcon />}
            disabled={!dirty || setMut.isPending}
            onClick={handleSave}
          >
            {setMut.isPending ? 'Saving…' : 'Save'}
          </Button>
          <Button
            size="small"
            variant="outlined"
            color="error"
            startIcon={<DeleteIcon />}
            disabled={!spec.configured || delMut.isPending}
            onClick={handleDelete}
          >
            Clear
          </Button>
        </Stack>
      </Stack>
      <TextField
        multiline
        fullWidth
        minRows={2}
        value={value}
        onChange={(e) => {
          setValue(e.target.value)
          setDirty(true)
        }}
        placeholder={isSecret ? '(secret — re-enter to update)' : 'JSON-encoded value'}
        InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.85rem' } }}
      />
    </Paper>
  )
}

const HelixOrgSettings: FC = () => {
  const { data, isLoading } = useHelixOrgSettings()
  const specs = data?.specs ?? []

  const { secretSpecs, plainSpecs } = useMemo(() => {
    const secret: SettingsSpecDTO[] = []
    const plain: SettingsSpecDTO[] = []
    for (const s of specs) {
      if (s.type.toLowerCase().includes('secret')) secret.push(s)
      else plain.push(s)
    }
    return { secretSpecs: secret, plainSpecs: plain }
  }, [specs])

  return (
    <Page breadcrumbTitle="Settings" breadcrumbParent={{ title: 'Helix Org' }}>
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Stack spacing={3}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Settings</Typography>
            <Typography variant="body2" color="text.secondary">
              Configuration registry for the helix-org runtime. Values are stored as raw JSON per spec type.
            </Typography>
          </Box>

          {data && (
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="caption" color="text.secondary">Operational state</Typography>
              <Stack direction="row" spacing={2} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
                <Chip size="small" label={`owner: ${data.owner || '—'}`} sx={{ fontFamily: 'monospace' }} />
                {data.public_url && (
                  <Chip size="small" label={`public_url: ${data.public_url}`} sx={{ fontFamily: 'monospace' }} />
                )}
                {data.db_path && (
                  <Chip size="small" label={`db: ${data.db_path}`} sx={{ fontFamily: 'monospace' }} />
                )}
                {data.envs_dir && (
                  <Chip size="small" label={`envs: ${data.envs_dir}`} sx={{ fontFamily: 'monospace' }} />
                )}
              </Stack>
            </Paper>
          )}

          {isLoading ? (
            <LoadingSpinner />
          ) : (
            <>
              {plainSpecs.length > 0 && (
                <Box>
                  <Typography variant="subtitle1" sx={{ mb: 1 }}>Configuration</Typography>
                  <Stack spacing={2}>
                    {plainSpecs.map((s) => (
                      <SettingRow key={s.key} spec={s} />
                    ))}
                  </Stack>
                </Box>
              )}
              {secretSpecs.length > 0 && (
                <Box>
                  <Typography variant="subtitle1" sx={{ mb: 1 }}>Secrets</Typography>
                  <Stack spacing={2}>
                    {secretSpecs.map((s) => (
                      <SettingRow key={s.key} spec={s} />
                    ))}
                  </Stack>
                </Box>
              )}
              {specs.length === 0 && (
                <Typography variant="body2" color="text.secondary">
                  No settings registered.
                </Typography>
              )}
            </>
          )}
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgSettings
