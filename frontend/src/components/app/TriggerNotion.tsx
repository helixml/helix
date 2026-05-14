import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import Circle from '@mui/icons-material/Circle'
import { TypesTrigger, TypesNotionTrigger, TypesNotionColumnMap } from '../../api/api'
import { IAppFlatState } from '../../types'
import { useListAppTriggers } from '../../services/appService'
import CopyButton from '../common/CopyButton'

interface TriggerNotionProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

// Generate a 32-char URL-safe random secret on the client when the user first
// enables the trigger. Stored in trigger config; pasted by the user into
// Notion's Database Automation "Custom headers" field as
// `X-Helix-Webhook-Secret`.
function randomSecret(): string {
  const arr = new Uint8Array(24)
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(arr)
  } else {
    for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256)
  }
  return Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('')
}

const TriggerNotion: FC<TriggerNotionProps> = ({
  appId,
  triggers = [],
  onUpdate,
  readOnly = false,
}) => {
  const existing = triggers.find(t => t.notion)
  const notion = existing?.notion as TypesNotionTrigger | undefined
  const enabled = !!notion?.enabled

  const { data: appTriggers, isLoading: isLoadingTriggers } = useListAppTriggers(appId, {
    enabled,
    refetchInterval: 5000,
  })
  const webhookURL = appTriggers?.data?.find(t => t.trigger?.notion)?.webhook_url

  const updateNotion = (next: Partial<TypesNotionTrigger>) => {
    const merged: TypesNotionTrigger = {
      ...(notion ?? {}),
      ...next,
    }
    if (existing) {
      onUpdate(triggers.map(t => (t.notion ? { ...t, notion: merged } : t)))
    } else {
      onUpdate([...triggers, { notion: merged }])
    }
  }

  const updateColumnMapping = (next: Partial<TypesNotionColumnMap>) => {
    updateNotion({ column_mapping: { ...(notion?.column_mapping ?? {}), ...next } })
  }

  const handleToggle = (on: boolean) => {
    if (on && !notion) {
      updateNotion({ enabled: true, shared_secret: randomSecret() })
    } else {
      updateNotion({ enabled: on })
    }
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box>
          <Typography gutterBottom>Notion</Typography>
          <Typography variant="body2" color="text.secondary">
            Trigger spectasks from Notion database rows. Connect a database and configure a Notion
            Database Automation that POSTs to Helix when a row's action column flips. See setup
            instructions below.
          </Typography>
        </Box>
        <FormControlLabel
          control={<Switch checked={enabled} onChange={e => handleToggle(e.target.checked)} disabled={readOnly} />}
          label=""
        />
      </Box>

      {enabled && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
            Webhook URL — paste into Notion Database Automation "Send webhook" → URL
          </Typography>
          <TextField
            fullWidth
            size="small"
            value={isLoadingTriggers ? 'Loading webhook URL…' : (webhookURL || 'Webhook URL appears once trigger is saved.')}
            InputProps={{
              readOnly: true,
              endAdornment: webhookURL ? <CopyButton content={webhookURL} title="Webhook URL" sx={{ mr: 0, mt: 0 }} /> : undefined,
            }}
            sx={{ mb: 2 }}
          />

          <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
            Shared secret — paste into Notion Database Automation "Custom headers" as
            <code style={{ marginLeft: 4 }}>X-Helix-Webhook-Secret: …</code>
          </Typography>
          <TextField
            fullWidth
            size="small"
            value={notion?.shared_secret || ''}
            InputProps={{
              readOnly: true,
              endAdornment: notion?.shared_secret ? <CopyButton content={notion.shared_secret} title="Shared secret" sx={{ mr: 0, mt: 0 }} /> : undefined,
            }}
            sx={{ mb: 2 }}
          />

          <Alert severity="info" sx={{ mb: 2 }}>
            Helix dispatches based on two more headers you set on the Notion Automation:
            <code> X-Helix-Source: notion-automation</code> (or <code>notion-button</code>), and
            <code> X-Helix-Action: create</code> (for the "Go" Automation) /
            <code> cancel</code> (for the "NoGo" Automation). Set up two Automations — one per direction.
          </Alert>

          <TextField
            fullWidth size="small" label="Target Helix project ID" sx={{ mb: 2 }}
            value={notion?.target_project_id || ''}
            onChange={e => updateNotion({ target_project_id: e.target.value })}
            disabled={readOnly}
          />
          <TextField
            fullWidth size="small" label="OAuth connection ID (Notion)" sx={{ mb: 2 }}
            value={notion?.oauth_connection_id || ''}
            onChange={e => updateNotion({ oauth_connection_id: e.target.value })}
            disabled={readOnly}
            helperText="Create the connection on the OAuth Connections page first; paste its ID here."
          />
          <TextField
            fullWidth size="small" label="Notion database ID" sx={{ mb: 2 }}
            value={notion?.notion_database_id || ''}
            onChange={e => updateNotion({ notion_database_id: e.target.value })}
            disabled={readOnly}
          />
          <TextField
            fullWidth size="small" label="Embed access token (Helix API key)" sx={{ mb: 2 }}
            value={notion?.embed_access_token || ''}
            onChange={e => updateNotion({ embed_access_token: e.target.value })}
            disabled={readOnly}
            helperText="API key used in the embed URL ?access_token= so viewers see the live Helix UI inside Notion."
          />

          <Typography variant="subtitle2" sx={{ mt: 2, mb: 1 }}>Column mapping</Typography>
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth size="small" label="Action column name (e.g. 'Go/NoGo')"
              value={notion?.column_mapping?.action_column || ''}
              onChange={e => updateColumnMapping({ action_column: e.target.value })}
              disabled={readOnly}
            />
            <TextField
              fullWidth size="small" label="Action column type"
              value={notion?.column_mapping?.action_column_type || 'select'}
              onChange={e => updateColumnMapping({ action_column_type: e.target.value })}
              disabled={readOnly}
              helperText="select or status"
            />
          </Box>
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth size="small" label="'Create' option (e.g. 'Go')"
              value={notion?.column_mapping?.action_option_create || ''}
              onChange={e => updateColumnMapping({ action_option_create: e.target.value })}
              disabled={readOnly}
            />
            <TextField
              fullWidth size="small" label="'Cancel' option (e.g. 'NoGo')"
              value={notion?.column_mapping?.action_option_cancel || ''}
              onChange={e => updateColumnMapping({ action_option_cancel: e.target.value })}
              disabled={readOnly}
            />
          </Box>
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth size="small" label="Prompt column (rich text, optional)"
              value={notion?.column_mapping?.prompt_column || ''}
              onChange={e => updateColumnMapping({ prompt_column: e.target.value })}
              disabled={readOnly}
            />
            <TextField
              fullWidth size="small" label="Result column (rich text, optional)"
              value={notion?.column_mapping?.result_column || ''}
              onChange={e => updateColumnMapping({ result_column: e.target.value })}
              disabled={readOnly}
            />
          </Box>

          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mt: 2 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Circle sx={{ fontSize: 12, color: webhookURL ? 'success.main' : 'grey.400' }} />
              <Typography variant="body2" color="text.secondary">
                <strong>Status:</strong>{' '}
                {webhookURL ? 'Notion integration active' : 'Save the trigger to generate the webhook URL'}
              </Typography>
            </Box>
            <Button
              variant="text"
              size="small"
              onClick={() =>
                window.open(
                  'https://github.com/helixml/helix/tree/helix-specs/design/tasks/002021_investigate-notion',
                  '_blank',
                )
              }
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

export default TriggerNotion
