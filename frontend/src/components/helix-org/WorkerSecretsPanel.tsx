import { FC, useMemo, useState } from 'react'
import { Alert, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, InputAdornment, ListItemText, MenuItem, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { Eye, EyeOff, KeyRound, Plus, Trash2 } from 'lucide-react'
import { TypesSecretScope, WorkersecretSourceKind } from '../../api/api'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { AvailableWorkerSecret, useAvailableWorkerSecrets, useDeleteWorkerSecret, usePutWorkerSecret, useWorkerSecrets } from '../../services/helixOrgService'

const WorkerSecretsPanel: FC<{ agentID?: string; projectID?: string; readOnly?: boolean }> = ({ agentID, projectID, readOnly = false }) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const { data: bindings = [] } = useWorkerSecrets(agentID)
  const { data: sources = [], refetch: refetchSources } = useAvailableWorkerSecrets(agentID)
  const put = usePutWorkerSecret(agentID)
  const del = useDeleteWorkerSecret(agentID)
  const [sourceID, setSourceID] = useState('')
  const source = useMemo(() => sources.find((item) => id(item) === sourceID), [sourceID, sources])
  const [name, setName] = useState('')
  const [usage, setUsage] = useState('')
  const [newSecretOpen, setNewSecretOpen] = useState(false)
  const [newSecretName, setNewSecretName] = useState('')
  const [newSecretValue, setNewSecretValue] = useState('')
  const [showSecretValue, setShowSecretValue] = useState(false)
  const [creatingSecret, setCreatingSecret] = useState(false)
  const newSecretNameTaken = bindings.some((binding) => binding.name === newSecretName.trim())

  if (!agentID) return null

  const selectSource = (value: string) => {
    setSourceID(value)
    const selected = sources.find((item) => id(item) === value)
    setName(selected?.proposed_name ?? '')
    setUsage(selected?.usage ?? '')
  }
  const grantSource = (selected: AvailableWorkerSecret, bindingName: string, bindingUsage: string) => put.mutateAsync({
    name: bindingName,
    payload: {
      source_kind: selected.source_kind,
      secret_id: selected.secret_id,
      account_id: selected.account_id,
      export_key: selected.export_key,
      usage: bindingUsage,
    },
  })
  const grant = async () => {
    if (!source || !name.trim()) return
    try {
      await grantSource(source, name.trim(), usage)
      setSourceID(''); setName(''); setUsage('')
      snackbar.success(`Granted ${name.trim()} to this Agent`)
    } catch (error: any) { snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to grant secret') }
  }
  const closeNewSecret = () => {
    if (creatingSecret) return
    setNewSecretOpen(false)
    setNewSecretName('')
    setNewSecretValue('')
    setShowSecretValue(false)
  }
  const createSecret = async () => {
    if (!projectID || !newSecretName.trim() || !newSecretValue) return
    setCreatingSecret(true)
    try {
      const created = (await api.getApiClient().v1ProjectsSecretsCreate(projectID, {
        name: newSecretName.trim(),
        value: newSecretValue,
        scope: TypesSecretScope.SecretScopeDev,
      })).data
      const refreshed = await refetchSources()
      const added = refreshed.data?.find((item) =>
        item.source_kind === WorkersecretSourceKind.SourceHelixSecret
        && (item.secret_id === created.id || item.label === created.name),
      )
      if (!added) throw new Error('Secret was created but is not available to this Agent')
      const bindingName = added.proposed_name?.trim() || created.name?.trim() || newSecretName.trim()
      await grantSource(added, bindingName, added.usage ?? '')
      setNewSecretOpen(false)
      setNewSecretName('')
      setNewSecretValue('')
      setShowSecretValue(false)
      setSourceID('')
      setName('')
      setUsage('')
      snackbar.success(`Secret created and granted as ${bindingName}`)
    } catch (error: any) {
      snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to create secret')
    } finally {
      setCreatingSecret(false)
    }
  }

  return <><Stack spacing={1.5}>
    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><KeyRound size={18}/><Typography variant="subtitle2">Secrets ({bindings.length})</Typography></Box>
      <Tooltip title={projectID ? 'Create a secret in this Agent’s project' : 'This Agent has no project'}><span><Button size="small" variant="outlined" startIcon={<Plus size={16}/>} onClick={() => setNewSecretOpen(true)} disabled={readOnly || !projectID}>New Secret</Button></span></Tooltip>
    </Box>
    <Alert severity="warning">Granting a binding authorizes this Agent to retrieve and use the credential value. Values are never shown on this page.</Alert>
    {bindings.map((binding) => <Box key={binding.name} sx={{ display: 'flex', alignItems: 'center', gap: 1, border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}>
      <Box sx={{ flex: 1, minWidth: 0 }}><Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontWeight: 600 }}>{binding.name}</Typography><Typography variant="caption" color="text.secondary">{binding.usage || binding.source_kind}</Typography></Box>
      <Tooltip title="Remove secret grant"><span><IconButton aria-label={`Remove ${binding.name}`} size="small" disabled={readOnly || del.isPending} onClick={async () => { try { await del.mutateAsync(binding.name!); snackbar.success(`Removed ${binding.name}`) } catch (error: any) { snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to remove secret') } }}><Trash2 size={18}/></IconButton></span></Tooltip>
    </Box>)}
    <TextField
      select
      size="small"
      label="Source"
      value={sourceID}
      onChange={(event) => selectSource(event.target.value)}
      disabled={readOnly}
      SelectProps={{ renderValue: (value) => workerSecretSourceLabel(sources.find((item) => id(item) === value)!) }}
    >
      {sources.map((item) => <MenuItem key={id(item)} value={id(item)} disabled={item.already_bound}><ListItemText primary={`${workerSecretSourceLabel(item)}${item.already_bound ? ' (granted)' : ''}`} secondary={`${item.group} · available as ${item.proposed_name}`} /></MenuItem>)}
    </TextField>
    {source && <><TextField size="small" label="Name" value={name} onChange={(event) => setName(event.target.value)} helperText="The stable name the Agent passes to get_secret" disabled={readOnly}/><TextField size="small" label="Usage" value={usage} onChange={(event) => setUsage(event.target.value)} disabled={readOnly}/><Button variant="outlined" onClick={() => { void grant() }} disabled={readOnly || !name.trim() || put.isPending}>Grant secret</Button></>}
    {sources.length === 0 && <Typography variant="caption" color="text.secondary">Create a project Secret or connect an organization account to make a source available.</Typography>}
  </Stack>
    <Dialog open={newSecretOpen} onClose={closeNewSecret} maxWidth="sm" fullWidth>
      <DialogTitle>New Project Secret</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <Typography variant="body2" color="text.secondary">This encrypted secret is stored in the Agent’s project and immediately granted to this Agent under the same name.</Typography>
          <TextField label="Secret name" value={newSecretName} onChange={(event) => setNewSecretName(event.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '_'))} placeholder="API_TOKEN" autoFocus required error={newSecretNameTaken} helperText={newSecretNameTaken ? 'This Agent already has a secret with this name.' : 'The Agent passes this name to get_secret.'} />
          <TextField
            label="Secret value"
            value={newSecretValue}
            onChange={(event) => setNewSecretValue(event.target.value)}
            type={showSecretValue ? 'text' : 'password'}
            required
            InputProps={{ endAdornment: <InputAdornment position="end"><Tooltip title={showSecretValue ? 'Hide value' : 'Show value'}><IconButton aria-label={showSecretValue ? 'Hide secret value' : 'Show secret value'} onClick={() => setShowSecretValue((shown) => !shown)} edge="end">{showSecretValue ? <EyeOff size={18}/> : <Eye size={18}/>}</IconButton></Tooltip></InputAdornment> }}
            helperText="Stored with dev scope for project sessions and agent access."
          />
        </Stack>
      </DialogContent>
      <DialogActions><Button onClick={closeNewSecret} disabled={creatingSecret}>Cancel</Button><Button variant="contained" color="secondary" startIcon={creatingSecret ? <CircularProgress size={16}/> : <Plus size={16}/>} onClick={() => { void createSecret() }} disabled={creatingSecret || newSecretNameTaken || !newSecretName.trim() || !newSecretValue}>{creatingSecret ? 'Creating and granting…' : 'Create and Grant'}</Button></DialogActions>
    </Dialog>
  </>
}

const id = (source: AvailableWorkerSecret) => source.source_kind === 'helix_secret' ? `secret:${source.secret_id}` : `account:${source.account_id}:${source.export_key}`

export const workerSecretSourceLabel = (source?: AvailableWorkerSecret) => {
  if (!source) return ''
  if (source.source_kind === 'helix_secret') return `Project secret — ${source.label}`
  const knownExports: Record<string, string> = {
    'slack_workspace/bot_token': 'Slack bot token',
    'github_app/installation_token': 'GitHub App token',
    'postmark/server_token': 'Postmark server token',
    'oauth/access_token': 'OAuth access token',
  }
  const credential = knownExports[source.export_key ?? ''] ?? (source.export_key ?? 'Connected account credential')
  return `${credential} — ${source.label}`
}

export default WorkerSecretsPanel
