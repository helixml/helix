import React, { useState } from 'react'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import { useUpdateSystemSettings } from '../../services/systemSettingsService'

export default function PayloadRetentionSetting({ days }: { days: number }) {
  const [draft, setDraft] = useState<string | null>(null)
  const save = useUpdateSystemSettings()
  const value = draft ?? String(days)
  const valid = /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= 3650
  return <Stack spacing={1} sx={{ mb: 2 }}>
    <Stack direction="row" alignItems="flex-start" spacing={1}>
      <TextField label="LLM payload retention (days)" size="small" value={value}
        error={!valid} inputProps={{ inputMode: 'numeric' }}
        helperText={valid ? 'Older request and response bodies expire permanently. Call metadata stays available.' : 'Enter 1 through 3650 days.'}
        onChange={e => setDraft(e.target.value)} />
      <Button disabled={!valid || Number(value) === days || save.isPending}
        onClick={() => save.mutate({ llm_payload_retention_days: Number(value) }, { onSuccess: () => setDraft(null) })}>Save</Button>
    </Stack>
    {save.isError && <Alert severity="error">Could not save payload retention.</Alert>}
  </Stack>
}
