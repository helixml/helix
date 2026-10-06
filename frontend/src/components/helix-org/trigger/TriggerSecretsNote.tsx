import { FC } from 'react'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import useRouter from '../../../hooks/useRouter'
import type { TransportSecretRef } from '../../../api/api'

const TriggerSecretsNote: FC<{ secrets?: TransportSecretRef[]; orgID?: string }> = ({ secrets, orgID }) => {
  const router = useRouter()
  if (!secrets?.length) return null

  return (
    <Stack spacing={0.75}>
      <Typography variant="caption" color="text.secondary">Credentials for this Trigger type</Typography>
      {secrets.map((secret) => (
        <Typography key={secret.label} variant="body2" color="text.secondary">
          {secret.label} — {secret.location}
          {secret.setting_key && (
            <Typography component="span" variant="caption" sx={{ ml: 0.5, fontFamily: 'monospace' }}>
              ({secret.setting_key})
            </Typography>
          )}
        </Typography>
      ))}
      {orgID && (
        <div>
          <Button size="small" variant="outlined" onClick={() => router.navigate('org_general', { org_id: orgID })}>
            Open Organization Settings
          </Button>
        </div>
      )}
    </Stack>
  )
}

export default TriggerSecretsNote
