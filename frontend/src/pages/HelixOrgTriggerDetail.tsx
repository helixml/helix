import { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Pencil } from 'lucide-react'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import TriggerFormDialog from '../components/helix-org/TriggerFormDialog'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useTrigger, useTriggerEvents, useUpdateTrigger } from '../services/triggerService'

const errorMessage = (error: any) => {
  const code = error?.response?.data?.code
  if (code === 'stale_resource') return 'This trigger changed in another window. Refresh it before saving again.'
  if (code === 'provider_connection_required') return 'Connect the provider in Organization Settings > Connected Accounts, then retry.'
  return error?.response?.data?.summary ?? error?.message ?? 'The request failed.'
}

const HelixOrgTriggerDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const id = router.params.trigger_id as string | undefined
  const { data: trigger, isLoading, error } = useTrigger(id)
  const { data: history } = useTriggerEvents(id)
  const update = useUpdateTrigger(id ?? '')
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Triggers', routeName: 'helix_org_triggers' })
  const [editing, setEditing] = useState(false)
  const [formError, setFormError] = useState('')

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle={trigger?.name ?? 'Trigger'}>
      <Box sx={{ height: '100%', overflow: 'auto' }}><Container maxWidth="lg" sx={{ py: 3 }}>
        {isLoading ? <LoadingSpinner /> : error || !trigger ? <Alert severity="info">Trigger not found. It may have been deleted or you may not have access.</Alert> : <Stack spacing={3}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
            <Box><Stack direction="row" spacing={1} alignItems="center"><Typography variant="h5">{trigger.name}</Typography><Chip size="small" label={trigger.kind} /></Stack><Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{trigger.description || 'No description'}</Typography><Typography variant="caption" color="text.secondary">{trigger.id}</Typography></Box>
            <Button variant="outlined" startIcon={<Pencil size={18} />} onClick={() => { setFormError(''); setEditing(true) }}>Edit</Button>
          </Stack>
          <Paper variant="outlined" sx={{ p: 2 }}><Typography variant="subtitle2" gutterBottom>Inbound configuration</Typography><Box component="pre" sx={{ m: 0, overflow: 'auto', whiteSpace: 'pre-wrap', color: 'text.secondary' }}>{JSON.stringify(trigger.config ?? {}, null, 2)}</Box></Paper>
          <Box><Typography variant="h6" gutterBottom>Event history</Typography><Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>Newest first · bounded to 50 events · refreshes automatically</Typography>
            {!history?.events?.length ? <Typography color="text.secondary">No events received yet.</Typography> : <Stack spacing={1}>{history.events.map((event) => <Paper key={event.id} variant="outlined" sx={{ p: 1.5 }}><Stack direction="row" justifyContent="space-between"><Typography variant="caption" color="text.secondary">{event.source || 'external'}</Typography><Typography variant="caption" color="text.secondary">{event.created_at ? new Date(event.created_at).toLocaleString() : ''}</Typography></Stack><Typography component="pre" variant="body2" sx={{ m: 0, mt: 1, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{event.body}</Typography></Paper>)}</Stack>}
          </Box>
        </Stack>}
      </Container></Box>
      {trigger && <TriggerFormDialog open={editing} trigger={trigger} saving={update.isPending} error={formError} onClose={() => setEditing(false)} onSubmit={async (payload) => { try { await update.mutateAsync(payload); setEditing(false); snackbar.success('Trigger updated') } catch (e) { setFormError(errorMessage(e)) } }} />}
    </HelixOrgShell>
  )
}

export default HelixOrgTriggerDetail
