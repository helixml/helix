import { FC, useMemo, useState } from 'react'
import { Alert, Box, Button, IconButton, MenuItem, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { KeyRound, Trash2 } from 'lucide-react'
import useSnackbar from '../../hooks/useSnackbar'
import { AvailableWorkerSecret, useAvailableWorkerSecrets, useDeleteWorkerSecret, usePutWorkerSecret, useWorkerSecrets } from '../../services/helixOrgService'

const WorkerSecretsPanel: FC<{ agentID?: string }> = ({ agentID }) => {
  const snackbar = useSnackbar()
  const { data: bindings = [] } = useWorkerSecrets(agentID)
  const { data: sources = [] } = useAvailableWorkerSecrets(agentID)
  const put = usePutWorkerSecret(agentID)
  const del = useDeleteWorkerSecret(agentID)
  const [sourceID, setSourceID] = useState('')
  const source = useMemo(() => sources.find((item) => id(item) === sourceID), [sourceID, sources])
  const [name, setName] = useState('')
  const [usage, setUsage] = useState('')

  if (!agentID) return null

  const selectSource = (value: string) => {
    setSourceID(value)
    const selected = sources.find((item) => id(item) === value)
    setName(selected?.proposed_name ?? '')
    setUsage(selected?.usage ?? '')
  }
  const grant = async () => {
    if (!source || !name.trim()) return
    try {
      await put.mutateAsync({ name: name.trim(), payload: { source_kind: source.source_kind, secret_id: source.secret_id, account_id: source.account_id, export_key: source.export_key, usage } })
      setSourceID(''); setName(''); setUsage('')
      snackbar.success(`Granted ${name.trim()} to this Agent`)
    } catch (error: any) { snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to grant secret') }
  }

  return <Stack spacing={1.5}>
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><KeyRound size={18}/><Typography variant="subtitle2">Secrets ({bindings.length})</Typography></Box>
    <Alert severity="warning">Granting a binding authorizes this Agent to retrieve and use the credential value. Values are never shown on this page.</Alert>
    {bindings.map((binding) => <Box key={binding.name} sx={{ display: 'flex', alignItems: 'center', gap: 1, border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}>
      <Box sx={{ flex: 1, minWidth: 0 }}><Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontWeight: 600 }}>{binding.name}</Typography><Typography variant="caption" color="text.secondary">{binding.usage || binding.source_kind}</Typography></Box>
      <Tooltip title="Remove secret grant"><IconButton aria-label={`Remove ${binding.name}`} size="small" onClick={async () => { try { await del.mutateAsync(binding.name!); snackbar.success(`Removed ${binding.name}`) } catch (error: any) { snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to remove secret') } }}><Trash2 size={18}/></IconButton></Tooltip>
    </Box>)}
    <TextField select size="small" label="Source" value={sourceID} onChange={(event) => selectSource(event.target.value)}>
      {sources.map((item) => <MenuItem key={id(item)} value={id(item)} disabled={item.already_bound}>{item.group}: {item.label}{item.already_bound ? ' (granted)' : ''}</MenuItem>)}
    </TextField>
    {source && <><TextField size="small" label="Name" value={name} onChange={(event) => setName(event.target.value)} helperText="The stable name the Agent passes to get_secret"/><TextField size="small" label="Usage" value={usage} onChange={(event) => setUsage(event.target.value)}/><Button variant="outlined" onClick={() => { void grant() }} disabled={!name.trim() || put.isPending}>Grant secret</Button></>}
    {sources.length === 0 && <Typography variant="caption" color="text.secondary">Create a project Secret or connect an organization account to make a source available.</Typography>}
  </Stack>
}

const id = (source: AvailableWorkerSecret) => source.source_kind === 'helix_secret' ? `secret:${source.secret_id}` : `account:${source.account_id}:${source.export_key}`
export default WorkerSecretsPanel
