import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import Alert from '@mui/material/Alert'
import Circle from '@mui/icons-material/Circle'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import { TypesTrigger } from '../../api/api'
import { IAppFlatState } from '../../types'
import { useListAppTriggers } from '../../services/appService'
import CopyButton from '../common/CopyButton'

import logo from '../../../assets/img/azure-devops/logo.png'

interface TriggerAzureDevOpsProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerAzureDevOps: FC<TriggerAzureDevOpsProps> = ({
  app,
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasAzureDevOpsTrigger = triggers.some(t => t.azure_devops && t.azure_devops.enabled === true)
  const azureDevOpsTrigger = triggers.find(t => t.azure_devops)?.azure_devops

  // Fetch app triggers to get webhook URL
  const { data: appTriggers, isLoading: isLoadingTriggers } = useListAppTriggers(appId, {
    enabled: hasAzureDevOpsTrigger,
    refetchInterval: 5000 // Refresh every 5 seconds when enabled
  })

  // Find the Azure DevOps webhook URL from the triggers
  const azureDevOpsWebhookUrl = appTriggers?.data?.find(
    trigger => trigger.trigger?.azure_devops
  )?.webhook_url

  const handleAzureDevOpsToggle = (enabled: boolean) => {
    // Find existing Azure DevOps trigger
    const existingAzureDevOpsTrigger = triggers.find(t => t.azure_devops)
    
    if (enabled) {
      if (existingAzureDevOpsTrigger) {
        // Update existing trigger to enabled
        const updatedTriggers = triggers.map(trigger => 
          trigger.azure_devops 
            ? { ...trigger, azure_devops: { ...trigger.azure_devops, enabled: true } }
            : trigger
        )
        onUpdate(updatedTriggers)
      } else {
        // Create new Azure DevOps trigger
        const newTriggers = [...triggers, { 
          azure_devops: { 
            enabled: true
          } 
        }]
        onUpdate(newTriggers)
      }
    } else {
      if (existingAzureDevOpsTrigger) {
        // Update existing trigger to disabled
        const updatedTriggers = triggers.map(trigger => 
          trigger.azure_devops 
            ? { ...trigger, azure_devops: { ...trigger.azure_devops, enabled: false } }
            : trigger
        )
        onUpdate(updatedTriggers)
      } else {
        // No Azure DevOps trigger exists, nothing to disable
        onUpdate(triggers)
      }
    }
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <Box
            component="img"
            src={logo}
            alt="Azure DevOps"
            sx={{ 
              mr: 2, 
              width: 24, 
              height: 24,
              objectFit: 'contain'
            }} 
          />
          <Box>
            <Typography gutterBottom>Azure DevOps</Typography>
            <Typography variant="body2" color="text.secondary">
              Connect your agent to Azure DevOps for pipeline triggers and notifications. Tuned to work with
              Pull Request (created, updated, commented) events.
            </Typography>
          </Box>
        </Box>
        <FormControlLabel
          control={
            <Switch
              checked={hasAzureDevOpsTrigger}
              onChange={(e) => handleAzureDevOpsToggle(e.target.checked)}
              disabled={readOnly}
            />
          }
          label=""
        />
      </Box>

      {(hasAzureDevOpsTrigger) && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1, opacity: hasAzureDevOpsTrigger ? 1 : 0.6 }}>
          {!hasAzureDevOpsTrigger && azureDevOpsTrigger && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Trigger is disabled. Enable it above to activate Azure DevOps integration.
            </Alert>
          )}
          
          {/* Webhook URL Display */}
          {hasAzureDevOpsTrigger && (
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
                Webhook URL
              </Typography>
              <TextField
                fullWidth
                size="small"
                placeholder="Webhook URL will be generated once the trigger is enabled..."
                value={isLoadingTriggers ? 'Loading webhook URL...' : (azureDevOpsWebhookUrl || '')}
                InputProps={{
                  readOnly: true,
                  endAdornment: azureDevOpsWebhookUrl ? (
                    <CopyButton
                      content={azureDevOpsWebhookUrl}
                      title="Webhook URL"
                      sx={{
                        mr: 0,
                        mt: 0
                      }}
                    />
                  ) : undefined
                }}
                disabled={readOnly || !hasAzureDevOpsTrigger}
                helperText="Use this URL as Service Hook destination in your Azure DevOps pipeline to trigger your agent"
                              />
            </Box>
          )}

          {/* Configuration summary */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {(() => {
                // If Azure DevOps trigger is disabled
                if (!azureDevOpsTrigger?.enabled) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Azure DevOps integration disabled
                      </Typography>
                    </>
                  )
                }

                // If we have a webhook URL, show green circle
                if (azureDevOpsWebhookUrl) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'success.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Azure DevOps integration active
                      </Typography>
                    </>
                  )
                }
                
                // If trigger is enabled but no webhook URL yet, show grey circle
                if (azureDevOpsTrigger?.enabled && !azureDevOpsWebhookUrl && !isLoadingTriggers) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Azure DevOps integration configured
                      </Typography>
                    </>
                  )
                }
                
                // Loading state
                return (
                  <>
                    <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                    <Typography variant="body2" color="text.secondary">
                      <strong>Status:</strong> Setting up Azure DevOps integration...
                    </Typography>
                  </>
                )
              })()}
            </Box>
            <Button
              variant="text"
              size="small"
              onClick={() => {
                // Open Azure DevOps documentation or setup instructions
                window.open('https://learn.microsoft.com/en-us/azure/devops/service-hooks/services/webhooks?view=azure-devops', '_blank')
              }}
              disabled={readOnly}
            >
              View setup instructions
            </Button>
          </Box>
        </Box>
      )}
    </Box>
  )
}

export default TriggerAzureDevOps
