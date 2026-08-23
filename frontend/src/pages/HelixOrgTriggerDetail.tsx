import { FC, useEffect, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CronScheduleFields from '../components/helix-org/CronScheduleFields'
import GitHubRepoPicker from '../components/helix-org/GitHubRepoPicker'
import { GitHubBranchesField } from '../components/helix-org/GitHubTopicConfigFields'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import TriggerWebhookPanel from '../components/helix-org/TriggerWebhookPanel'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import CopyButtonWithCheck from '../components/session/CopyButtonWithCheck'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { TriggerDTO, useTrigger, useTriggerEvents, useUpdateTrigger } from '../services/triggerService'

const errorMessage = (error: any) => {
  const code = error?.response?.data?.code
  if (code === 'stale_resource') return 'This trigger changed in another window. Refresh it before saving again.'
  if (code === 'provider_connection_required') return 'Connect the provider in Organization Settings > Connected Accounts, then retry.'
  return error?.response?.data?.summary ?? error?.message ?? 'The request failed.'
}

const TriggerConfiguration: FC<{ trigger: TriggerDTO }> = ({ trigger }) => {
  const snackbar = useSnackbar()
  const update = useUpdateTrigger(trigger.id ?? '')
  const [name, setName] = useState(trigger.name ?? '')
  const [description, setDescription] = useState(trigger.description ?? '')
  const [config, setConfig] = useState(JSON.stringify(trigger.config ?? {}, null, 2))
  const initialConfig = (trigger.config ?? {}) as Record<string, unknown>
  const [cronSchedule, setCronSchedule] = useState(typeof initialConfig.schedule === 'string' ? initialConfig.schedule : '')
  const [cronMessage, setCronMessage] = useState(typeof initialConfig.message === 'string' ? initialConfig.message : '')
  const [ghRepo, setGhRepo] = useState(typeof initialConfig.repo === 'string' ? initialConfig.repo : '')
  const [ghBranches, setGhBranches] = useState<string[]>(Array.isArray(initialConfig.branches) ? initialConfig.branches as string[] : ['*'])
  const [slackChannel, setSlackChannel] = useState(typeof initialConfig.channel_id === 'string' ? initialConfig.channel_id : '')

  useEffect(() => {
    setName(trigger.name ?? '')
    setDescription(trigger.description ?? '')
    setConfig(JSON.stringify(trigger.config ?? {}, null, 2))
    const next = (trigger.config ?? {}) as Record<string, unknown>
    setCronSchedule(typeof next.schedule === 'string' ? next.schedule : '')
    setCronMessage(typeof next.message === 'string' ? next.message : '')
    setGhRepo(typeof next.repo === 'string' ? next.repo : '')
    setGhBranches(Array.isArray(next.branches) ? next.branches as string[] : ['*'])
    setSlackChannel(typeof next.channel_id === 'string' ? next.channel_id : '')
  }, [trigger.description, trigger.id, trigger.name, trigger.revision])

  const savedConfig = JSON.stringify(trigger.config ?? {}, null, 2)
  const currentConfig = () => {
    if (trigger.kind === 'cron') return { schedule: cronSchedule.trim(), ...(cronMessage.trim() ? { message: cronMessage.trim() } : {}) }
    if (trigger.kind === 'github') return { ...initialConfig, repo: ghRepo.trim(), branches: ghBranches.map((branch) => branch.trim()).filter(Boolean) }
    if (trigger.kind === 'slack') return { ...initialConfig, channel_id: slackChannel.trim() }
    return undefined
  }
  const guidedConfig = currentConfig()
  const savedGuidedConfig = trigger.kind === 'cron'
    ? { schedule: typeof initialConfig.schedule === 'string' ? initialConfig.schedule : '', ...(typeof initialConfig.message === 'string' && initialConfig.message ? { message: initialConfig.message } : {}) }
    : trigger.kind === 'github'
      ? { ...initialConfig, repo: typeof initialConfig.repo === 'string' ? initialConfig.repo : '', branches: Array.isArray(initialConfig.branches) ? initialConfig.branches : ['*'] }
      : trigger.kind === 'slack'
        ? { ...initialConfig, channel_id: typeof initialConfig.channel_id === 'string' ? initialConfig.channel_id : '' }
        : undefined
  const dirty = name !== trigger.name || description !== (trigger.description ?? '') || (guidedConfig ? JSON.stringify(guidedConfig) !== JSON.stringify(savedGuidedConfig) : config !== savedConfig)
  const reset = () => {
    setName(trigger.name ?? '')
    setDescription(trigger.description ?? '')
    setConfig(savedConfig)
    const saved = (trigger.config ?? {}) as Record<string, unknown>
    setCronSchedule(typeof saved.schedule === 'string' ? saved.schedule : '')
    setCronMessage(typeof saved.message === 'string' ? saved.message : '')
    setGhRepo(typeof saved.repo === 'string' ? saved.repo : '')
    setGhBranches(Array.isArray(saved.branches) ? saved.branches as string[] : ['*'])
    setSlackChannel(typeof saved.channel_id === 'string' ? saved.channel_id : '')
  }
  const save = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    let parsed = currentConfig()
    if (trigger.kind === 'cron' && !cronSchedule.trim()) return snackbar.error('Schedule is required')
    if (trigger.kind === 'github' && !ghRepo.trim()) return snackbar.error('GitHub repository is required')
    if (!parsed) {
      try {
        parsed = JSON.parse(config || '{}')
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error()
      } catch {
        snackbar.error('Trigger settings must be a JSON object')
        return
      }
    }
    try {
      await update.mutateAsync({ name: name.trim(), description: description.trim(), kind: trigger.kind ?? '', config: parsed, revision: trigger.revision })
      snackbar.success('Trigger updated')
    } catch (error) {
      snackbar.error(errorMessage(error))
    }
  }

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>Configuration</Typography>
      <Stack spacing={2}>
        <TextField label="Name" value={name} onChange={(event) => setName(event.target.value)} size="small" required />
        <TextField label="Description (optional)" value={description} onChange={(event) => setDescription(event.target.value)} size="small" multiline minRows={2} />
        {trigger.kind === 'github' && <><GitHubRepoPicker value={ghRepo} onChange={setGhRepo} /><GitHubBranchesField branches={ghBranches} onChange={setGhBranches} /></>}
        {trigger.kind === 'cron' && <CronScheduleFields value={cronSchedule} onChange={setCronSchedule} message={cronMessage} onMessageChange={setCronMessage} />}
        {trigger.kind === 'slack' && <TextField label="Slack channel ID (optional)" value={slackChannel} onChange={(event) => setSlackChannel(event.target.value)} size="small" placeholder="C012ABCDEF" helperText="Limit this Trigger to one Slack channel. Leave empty to receive events from the whole connected workspace." />}
        {trigger.kind !== 'local' && trigger.kind !== 'helix_events' && trigger.kind !== 'github' && trigger.kind !== 'cron' && trigger.kind !== 'slack' && (
          <TextField label="Trigger settings" value={config} onChange={(event) => setConfig(event.target.value)} size="small" multiline minRows={5} sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.8rem' } }} />
        )}
        {(trigger.kind === 'local' || trigger.kind === 'helix_events') && <Typography variant="caption" color="text.secondary">This Trigger type has no additional settings.</Typography>}
      </Stack>
      {dirty && <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ mt: 2, pt: 2, borderTop: '1px solid', borderColor: 'divider' }}><Button variant="contained" color="secondary" size="small" onClick={save} disabled={update.isPending}>{update.isPending ? 'Saving…' : 'Save'}</Button><Button size="small" onClick={reset} disabled={update.isPending}>Cancel</Button></Stack>}
    </Box>
  )
}

const HelixOrgTriggerDetail: FC = () => {
  const router = useRouter()
  const id = router.params.trigger_id as string | undefined
  const { data: trigger, isLoading, error } = useTrigger(id)
  const { data: history } = useTriggerEvents(id)
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Triggers', routeName: 'helix_org_triggers' })

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle={trigger?.name ?? 'Trigger'}>
      <Box sx={{ height: '100%', overflow: 'auto' }}><Container maxWidth="lg" sx={{ py: 3 }}>
        {isLoading ? <LoadingSpinner /> : error || !trigger ? <Alert severity="info">Trigger not found. It may have been deleted or you may not have access.</Alert> : <Stack spacing={3}>
          <Box><Stack direction="row" spacing={1} alignItems="center"><Typography variant="h5" sx={{ fontFamily: 'monospace' }}>{trigger.id}</Typography><CopyButtonWithCheck text={trigger.id ?? ''} /><Chip size="small" label={trigger.kind} /></Stack><Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{trigger.name}</Typography>{trigger.description && <Typography variant="body2" sx={{ mt: 1 }}>{trigger.description}</Typography>}<Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>{trigger.created_at ? new Date(trigger.created_at).toLocaleString() : ''}</Typography></Box>
          <Divider />
          <TriggerConfiguration trigger={trigger} />
          {trigger.kind === 'github' && <TriggerWebhookPanel trigger={trigger} orgSlug={router.params.org_id as string} />}
          <Divider />
          <Box>
            <Typography variant="h6" gutterBottom>Agents started</Typography>
            {!trigger.attached_workers?.length
              ? <Typography variant="body2" color="text.secondary">No agents are attached to this Trigger yet. Attach one from the org chart.</Typography>
              : <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                  {trigger.attached_workers.map((workerID) => (
                    <Chip
                      key={workerID}
                      size="small"
                      label={workerID}
                      onClick={() => router.navigate('helix_org_bot_detail', { org_id: router.params.org_id as string, bot_id: workerID })}
                      sx={{ fontFamily: 'monospace' }}
                    />
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
