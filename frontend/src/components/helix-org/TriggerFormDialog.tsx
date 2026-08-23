import { FC, useEffect, useRef, useState } from 'react'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import Typography from '@mui/material/Typography'
import { TypesExternalRepositoryType } from '../../api/api'
import useAccount from '../../hooks/useAccount'
import { useGitRepositories } from '../../services/gitRepositoryService'
import { useGitHubAppInstallation, useListGitHubRepos } from '../../services/helixOrgService'
import { TriggerDTO, TriggerWriteRequest } from '../../services/triggerService'
import { getUserTimezone } from '../../utils/cronUtils'
import CronScheduleFields, { buildSpecificTimeCron } from './CronScheduleFields'
import { GitHubAppConnect } from './GitHubAppPanel'
import GitHubRepoPicker from './GitHubRepoPicker'
import { GitHubBranchesField, GitHubEventsField } from './GitHubTopicConfigFields'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'

const KINDS = [
  { value: 'local', label: 'Manual or agent event', help: 'Agents and Helix can fire this Trigger directly.' },
  { value: 'github', label: 'GitHub event', help: 'Run agents when selected events happen in a GitHub repository.' },
  { value: 'gitlab', label: 'GitLab event', help: 'Run agents when merge request events happen in a GitLab repository.' },
  { value: 'email', label: 'Incoming email', help: 'Run agents when email arrives at a configured address.' },
  { value: 'cron', label: 'Schedule', help: 'Run agents automatically on a schedule.' },
  { value: 'webhook', label: 'Webhook', help: 'Run agents when an external system sends a webhook.' },
  { value: 'slack', label: 'Slack event', help: 'Run agents when a configured Slack event arrives.' },
]

const defaultCronSchedule = () => buildSpecificTimeCron([1, 2, 3, 4, 5], 9, 0, getUserTimezone())

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
  const [ghRepo, setGhRepo] = useState('')
  const [ghEvents, setGhEvents] = useState<string[]>(['*'])
  const [ghBranches, setGhBranches] = useState<string[]>(['*'])
  const [glRepositoryID, setGlRepositoryID] = useState('')
  const [cronSchedule, setCronSchedule] = useState(defaultCronSchedule)
  const [cronMessage, setCronMessage] = useState('')
  const [emailAddress, setEmailAddress] = useState('')
  const [fieldError, setFieldError] = useState('')
  const account = useAccount()
  const repositories = useGitRepositories({ organizationId: account.organizationTools.organization?.id, enabled: open })
  const gitLabRepositories = (repositories.data ?? []).filter((repo) => repo.external_type === TypesExternalRepositoryType.ExternalRepositoryTypeGitLab)
  const ghReposQuery = useListGitHubRepos({ enabled: open })
  const ghInstallQuery = useGitHubAppInstallation({ enabled: open, pollWhileNotInstalled: open })
  const ghInstalled = !ghInstallQuery.isLoading && ghInstallQuery.data?.installed === true
  const prevInstalledRef = useRef(false)

  useEffect(() => {
    if (ghInstalled && !prevInstalledRef.current) ghReposQuery.refetch()
    prevInstalledRef.current = ghInstalled
    // The query object is intentionally excluded; installation state is the event.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ghInstalled])

  useEffect(() => {
    if (!open) return
    setName(trigger?.name ?? '')
    setDescription(trigger?.description ?? '')
    setKind(trigger?.kind ?? 'local')
    setConfig(JSON.stringify(trigger?.config ?? {}, null, 2))
    setGhRepo('')
    setGhEvents(['*'])
    setGhBranches(['*'])
    setGlRepositoryID('')
    setCronSchedule(defaultCronSchedule())
    setCronMessage('')
    setEmailAddress('')
    setFieldError('')
  }, [open, trigger?.id])

  const submit = async () => {
    if (!name.trim()) {
      setFieldError('Name is required.')
      return
    }
    let parsed: Record<string, unknown> = {}
    if (!trigger && kind === 'github') {
      if (!ghInstalled || !ghRepo.trim()) return setFieldError('Install the GitHub App and select a repository.')
      parsed = { repo: ghRepo.trim(), events: ghEvents.length ? ghEvents : ['*'], branches: ghBranches.length ? ghBranches : ['*'] }
    } else if (!trigger && kind === 'gitlab') {
      const repo = gitLabRepositories.find((item) => item.id === glRepositoryID)
      if (!repo?.external_url) return setFieldError('Select a GitLab repository.')
      parsed = { repo: repo.external_url, repository_id: repo.id, events: ['Merge Request Hook'] }
    } else if (!trigger && kind === 'cron') {
      if (!cronSchedule.trim()) return setFieldError('Schedule is required.')
      parsed = { schedule: cronSchedule.trim(), ...(cronMessage.trim() ? { message: cronMessage.trim() } : {}) }
    } else if (!trigger && kind === 'email') {
      if (!emailAddress.trim()) return setFieldError('Email address is required.')
      parsed = { inbound_address: emailAddress.trim() }
    } else {
      try {
        parsed = JSON.parse(config || '{}')
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error()
      } catch {
        setFieldError('Trigger settings must be a JSON object.')
        return
      }
    }
    setFieldError('')
    await onSubmit({ name: name.trim(), description: description.trim(), kind, config: parsed, revision: trigger?.revision })
  }

  return (
    <HelixOrgSideDrawer open={open} onClose={saving ? () => undefined : onClose} title={trigger ? 'Edit Trigger' : 'New Trigger'} width={480}>
        <Stack spacing={2}>
          <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus size="small" />
          <TextField label="Description (optional)" value={description} onChange={(e) => setDescription(e.target.value)} multiline minRows={2} size="small" />
          {(error || fieldError) && <Alert severity="error">{fieldError || error}</Alert>}
          <FormControl fullWidth size="small">
            <InputLabel id="trigger-kind-label">What starts this Trigger?</InputLabel>
            <Select labelId="trigger-kind-label" label="What starts this Trigger?" value={kind} onChange={(e) => { setKind(e.target.value); setConfig('{}'); setFieldError('') }} disabled={!!trigger}>
              {KINDS.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
            </Select>
          </FormControl>
          {!trigger && <Typography variant="caption" color="text.secondary">{KINDS.find((option) => option.value === kind)?.help}</Typography>}
          {!trigger && kind === 'github' && <>
            {!ghInstalled && <Typography variant="body2" color="text.secondary">Connect the Helix GitHub App, then choose the repository that should start this Trigger.</Typography>}
            <GitHubAppConnect mode="gate" onChange={() => { ghInstallQuery.refetch(); ghReposQuery.refetch() }} />
            {ghInstalled && <><GitHubRepoPicker value={ghRepo} onChange={setGhRepo} enabled={open} /><GitHubEventsField events={ghEvents} onChange={setGhEvents} /><GitHubBranchesField branches={ghBranches} onChange={setGhBranches} /></>}
          </>}
          {!trigger && kind === 'gitlab' && <FormControl fullWidth size="small"><InputLabel id="trigger-gitlab-repo-label">GitLab repository</InputLabel><Select labelId="trigger-gitlab-repo-label" label="GitLab repository" value={glRepositoryID} onChange={(e) => setGlRepositoryID(e.target.value)}>{gitLabRepositories.map((repo) => <MenuItem key={repo.id} value={repo.id}>{repo.external_url || repo.name}</MenuItem>)}</Select></FormControl>}
          {!trigger && kind === 'cron' && <CronScheduleFields key={open ? 'cron-open' : 'cron-closed'} value={cronSchedule} onChange={setCronSchedule} message={cronMessage} onMessageChange={setCronMessage} defaultMode="specific_time" />}
          {!trigger && kind === 'email' && <TextField label="Incoming email address" placeholder="inbox@example.com" value={emailAddress} onChange={(e) => setEmailAddress(e.target.value)} size="small" helperText="Email sent to this address starts the attached agents." />}
          {((!trigger && (kind === 'webhook' || kind === 'slack')) || (trigger && kind !== 'local' && kind !== 'helix_events')) && (
            <TextField
              label="Trigger settings"
              value={config}
              onChange={(e) => setConfig(e.target.value)}
              multiline
              minRows={6}
              size="small"
              helperText="Advanced settings only. Provider credentials are managed in Organization Settings > Connected Accounts."
            />
          )}
          <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
            <Button variant="contained" color="secondary" onClick={submit} disabled={saving}>{saving ? 'Saving…' : trigger ? 'Save' : 'Create'}</Button>
            <Button onClick={onClose} disabled={saving}>Cancel</Button>
          </Stack>
        </Stack>
    </HelixOrgSideDrawer>
  )
}

export default TriggerFormDialog
