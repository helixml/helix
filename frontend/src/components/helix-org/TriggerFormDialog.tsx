import { FC, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import { TriggerDTO, TriggerWriteRequest } from '../../services/triggerService'

const KINDS = ['local', 'webhook', 'email', 'github', 'gitlab', 'cron', 'slack', 'helix_events']

interface Props {
  open: boolean
  trigger?: TriggerDTO
  saving: boolean
  error?: string
  onClose: () => void
  onSubmit: (payload: TriggerWriteRequest) => Promise<void>
}

const TriggerFormDialog: FC<Props> = ({ open, trigger, saving, error, onClose, onSubmit }) => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [kind, setKind] = useState('local')
  const [config, setConfig] = useState('{}')
  const [fieldError, setFieldError] = useState('')

  useEffect(() => {
    if (!open) return
    setName(trigger?.name ?? '')
    setDescription(trigger?.description ?? '')
    setKind(trigger?.kind ?? 'local')
    setConfig(JSON.stringify(trigger?.config ?? {}, null, 2))
    setFieldError('')
  }, [open, trigger?.id])

  const submit = async () => {
    if (!name.trim()) {
      setFieldError('Name is required.')
      return
    }
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(config || '{}')
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error()
    } catch {
      setFieldError('Configuration must be a JSON object.')
      return
    }
    setFieldError('')
    await onSubmit({ name: name.trim(), description: description.trim(), kind, config: parsed, revision: trigger?.revision })
  }

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} fullWidth maxWidth="sm">
      <DialogTitle>{trigger ? 'Edit trigger' : 'New trigger'}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          {(error || fieldError) && <Alert severity="error">{fieldError || error}</Alert>}
          <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
          <TextField label="Description" value={description} onChange={(e) => setDescription(e.target.value)} multiline minRows={2} />
          <TextField select label="Trigger type" value={kind} onChange={(e) => { setKind(e.target.value); setConfig('{}') }} disabled={!!trigger}>
            {KINDS.map((value) => <MenuItem key={value} value={value}>{value.replace('_', ' ')}</MenuItem>)}
          </TextField>
          {kind !== 'local' && kind !== 'helix_events' && (
            <TextField
              label="Trigger settings"
              value={config}
              onChange={(e) => setConfig(e.target.value)}
              multiline
              minRows={6}
              helperText="Provider credentials are configured in Organization Settings > Connected Accounts and are never stored here."
            />
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button variant="contained" onClick={submit} disabled={saving}>{trigger ? 'Save' : 'Create trigger'}</Button>
      </DialogActions>
    </Dialog>
  )
}

export default TriggerFormDialog
