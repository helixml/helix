import { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'

import { TriggerDTO, TriggerWriteRequest } from '../../services/triggerService'
import { GitHubAppConnect } from './GitHubAppPanel'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'
import TriggerConfig, { TriggerConfigValue } from './trigger/TriggerConfig'

interface Props {
  open: boolean
  trigger?: TriggerDTO
  saving: boolean
  error?: string
  orgID?: string
  onClose: () => void
  onSubmit: (payload: TriggerWriteRequest) => Promise<void>
}

const TriggerFormDialog: FC<Props> = ({ open, trigger, saving, error, orgID, onClose, onSubmit }) => {
  const [value, setValue] = useState<TriggerConfigValue>()
  const [valid, setValid] = useState(false)
  const [fieldError, setFieldError] = useState('')

  const submit = async () => {
    if (!value?.name.trim()) return setFieldError('Name is required.')
    if (!valid) return setFieldError('Fill in every required field.')
    setFieldError('')
    await onSubmit({ ...value, revision: trigger?.revision })
  }

  return (
    <HelixOrgSideDrawer
      open={open}
      onClose={saving ? () => undefined : onClose}
      title={trigger ? 'Edit Trigger' : 'New Trigger'}
      width={480}
    >
      <Stack spacing={2}>
        {(error || fieldError) && <Alert severity="error">{fieldError || error}</Alert>}
        {value?.kind === 'github' && <GitHubAppConnect mode="gate" onChange={() => undefined} />}
        <TriggerConfig
          key={open ? `${trigger?.id ?? 'new'}-open` : 'closed'}
          trigger={trigger}
          density="full"
          mode={trigger ? 'edit' : 'create'}
          orgID={orgID}
          onChange={(next, isValid) => { setValue(next); setValid(isValid) }}
        />
        <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
          <Button variant="contained" color="secondary" onClick={submit} disabled={saving}>
            {saving ? 'Saving…' : trigger ? 'Save' : 'Create'}
          </Button>
          <Button onClick={onClose} disabled={saving}>Cancel</Button>
        </Stack>
      </Stack>
    </HelixOrgSideDrawer>
  )
}

export default TriggerFormDialog
