import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import ApiIcon from '@mui/icons-material/Api'
import { TypesTrigger } from '../../api/api'
import TriggerCron from './TriggerCron'
import TriggerSlack from './TriggerSlack'
import TriggerTeams from './TriggerTeams'
import TriggerAzureDevOps from './TriggerAzureDevOps'
import TriggerCrisp from './TriggerCrisp'
import { IAppFlatState } from '../../types'

interface TriggersProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const Triggers: FC<TriggersProps> = ({
  app,
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  return (
    <Box sx={{ mt: 2, mr: 2}}>
      <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
        Triggers
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Choose how you want to interact with your agent. Direct API requests, recurring schedules, or 3rd party applications.
      </Typography>

      {/* Sessions/API Trigger - Always enabled and read-only */}
      <Box sx={{ mb: 3, p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <ApiIcon sx={{ mr: 2, color: 'primary.main' }} />
            <Box>
              <Typography gutterBottom>Sessions & API</Typography>
              <Typography variant="body2" color="text.secondary">
                Allow users to interact with your agent through the web interface or API
              </Typography>
            </Box>
          </Box>
          <FormControlLabel
            control={<Switch checked={true} disabled />}
            label=""
          />
        </Box>
      </Box>

      {/* Recurring Trigger */}
      <TriggerCron 
        triggers={triggers}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />

      {/* Add spacing between triggers */}
      <Box sx={{ my: 3 }} />

      {/* Slack Trigger */}
      <TriggerSlack
        app={app}
        appId={appId}
        triggers={triggers}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />

      {/* Add spacing between triggers */}
      <Box sx={{ my: 3 }} />

      {/* Teams Trigger */}
      <TriggerTeams
        app={app}
        appId={appId}
        triggers={triggers}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />

      {/* Add spacing between triggers */}
      <Box sx={{ my: 3 }} />

      {/* Azure DevOps Trigger */}
      <TriggerAzureDevOps
        app={app}
        appId={appId}
        triggers={triggers}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />

      {/* Add spacing between triggers */}
      <Box sx={{ my: 3 }} />

      {/* Crisp Trigger */}
      <TriggerCrisp
        app={app}
        appId={appId}
        triggers={triggers}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />
    </Box>
  )
}

export default Triggers
