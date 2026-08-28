import { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import TriggerWebhookPanel from '../components/helix-org/TriggerWebhookPanel'
import TriggerConfig, { TriggerConfigValue } from '../components/helix-org/trigger/TriggerConfig'
import { configEquals } from '../components/helix-org/trigger/triggerConfigModel'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import CopyButtonWithCheck from '../components/session/CopyButtonWithCheck'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useListHelixOrgBots } from '../services/helixOrgService'
import { useTriggerKind } from '../services/triggerKindService'
import { TriggerDTO, useTrigger, useTriggerEvents, useUpdateTrigger } from '../services/triggerService'

const errorMessage = (error: any) => {
  const code = error?.response?.data?.code
  if (code === 'stale_resource') return 'This trigger changed in another window. Refresh it before saving again.'
  if (code === 'provider_connection_required') return 'Connect the provider in Organization Settings > Connected Accounts, then retry.'
  return error?.response?.data?.summary ?? error?.message ?? 'The request failed.'
}

const TriggerConfiguration: FC<{ trigger: TriggerDTO; orgID: string }> = ({ trigger, orgID }) => {
  const snackbar = useSnackbar()
  const update = useUpdateTrigger(trigger.id ?? '')
  const [value, setValue] = useState<TriggerConfigValue>()
  const [valid, setValid] = useState(true)
  // Bumped by Cancel to remount TriggerConfig, which owns the draft.
  const [resetKey, setResetKey] = useState(0)

  const dirty = !!value && !(
    value.name === (trigger.name ?? '') &&
    value.description === (trigger.description ?? '') &&
    configEquals(value.config, trigger.config ?? {})
  )

  const save = async () => {
    if (!value) return
    if (!valid) {
      snackbar.error('Fill in every required field before saving')
      return
    }
    try {
      await update.mutateAsync({ ...value, kind: trigger.kind ?? '', revision: trigger.revision })
      snackbar.success('Trigger updated')
    } catch (error) {
      snackbar.error(errorMessage(error))
    }
  }

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>Configuration</Typography>
      <TriggerConfig
        key={`${trigger.revision}-${resetKey}`}
        trigger={trigger}
        density="full"
        mode="edit"
        orgID={orgID}
        onChange={(next, isValid) => { setValue(next); setValid(isValid) }}
      />
      {dirty && (
        <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ mt: 2, pt: 2, borderTop: '1px solid', borderColor: 'divider' }}>
          <Button variant="contained" color="secondary" size="small" onClick={save} disabled={update.isPending}>
            {update.isPending ? 'Saving…' : 'Save'}
          </Button>
          <Button size="small" onClick={() => { setValue(undefined); setResetKey((n) => n + 1) }} disabled={update.isPending}>
            Cancel
          </Button>
        </Stack>
      )}
    </Box>
  )
}

const HelixOrgTriggerDetail: FC = () => {
  const router = useRouter()
  const orgID = router.params.org_id as string
  const id = router.params.trigger_id as string | undefined
  const { data: trigger, isLoading, error } = useTrigger(id)
  const { data: history } = useTriggerEvents(id)
  const { data: bots = [] } = useListHelixOrgBots()
  const kindDescriptor = useTriggerKind(trigger?.kind)
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Triggers', routeName: 'helix_org_triggers' })

  const botLabel = (workerID: string) => bots.find((bot) => bot.id === workerID)?.name || workerID

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle={trigger?.name ?? 'Trigger'}>
      <Box sx={{ height: '100%', overflow: 'auto' }}><Container maxWidth="lg" sx={{ py: 3 }}>
        {isLoading ? <LoadingSpinner /> : error || !trigger ? <Alert severity="info">Trigger not found. It may have been deleted or you may not have access.</Alert> : <Stack spacing={3}>
          <Box>
            <Stack direction="row" spacing={1} alignItems="center">
              <Typography variant="h5">{trigger.name}</Typography>
              <Chip size="small" label={kindDescriptor?.label ?? trigger.kind} />
            </Stack>
            {trigger.description && <Typography variant="body2" sx={{ mt: 1 }}>{trigger.description}</Typography>}
            <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 1 }}>
              <Typography variant="caption" color="text.secondary">ID</Typography>
              <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>{trigger.id}</Typography>
              <CopyButtonWithCheck text={trigger.id ?? ''} />
              <Typography variant="caption" color="text.secondary">
                · created {trigger.created_at ? new Date(trigger.created_at).toLocaleString() : ''}
              </Typography>
            </Stack>
          </Box>
          <Divider />
          <TriggerConfiguration trigger={trigger} orgID={orgID} />
          {trigger.kind === 'github' && <TriggerWebhookPanel trigger={trigger} orgSlug={orgID} />}
          <Divider />
          <Box>
            <Typography variant="h6" gutterBottom>Agents started</Typography>
            {!trigger.attached_workers?.length
              ? <Typography variant="body2" color="text.secondary">No agents are attached to this Trigger yet. Attach one from the org chart.</Typography>
              : <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                  {trigger.attached_workers.map((workerID) => (
                    <Tooltip key={workerID} title={workerID}>
                      <Chip
                        size="small"
                        label={botLabel(workerID)}
                        onClick={() => router.navigate('helix_org_bot_detail', { org_id: orgID, bot_id: workerID })}
                      />
                    </Tooltip>
                  ))}
                </Stack>}
          </Box>
          <Divider />
          <Box><Typography variant="h6" gutterBottom>Event history</Typography><Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>Newest first · bounded to 50 events · refreshes automatically</Typography>
            {!history?.events?.length ? <Typography color="text.secondary">No events received yet.</Typography> : <Stack spacing={1}>{history.events.map((event) => <Paper key={event.id} variant="outlined" sx={{ p: 1.5 }}><Stack direction="row" justifyContent="space-between"><Typography variant="caption" color="text.secondary">{event.source || 'external'}</Typography><Typography variant="caption" color="text.secondary">{event.created_at ? new Date(event.created_at).toLocaleString() : ''}</Typography></Stack><Typography component="pre" variant="body2" sx={{ m: 0, mt: 1, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{event.body}</Typography></Paper>)}</Stack>}
          </Box>
        </Stack>}
      </Container></Box>
    </HelixOrgShell>
  )
}

export default HelixOrgTriggerDetail
