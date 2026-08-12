import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Divider from '@mui/material/Divider'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import ListItemText from '@mui/material/ListItemText'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import DnsOutlinedIcon from '@mui/icons-material/DnsOutlined'

import { AssetAuthType, AssetKind } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import {
  AssetDTO,
  AssetHealthDTO,
  BotDTO,
  useCreateAsset,
  useLinkAsset,
  useUnlinkAsset,
  useUpdateAsset,
} from '../../services/helixOrgService'
import CopyButtonWithCheck from '../session/CopyButtonWithCheck'
import HelixOrgOverviewCard from './HelixOrgOverviewCard'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'

type AssetForm = {
  name: string
  description: string
  address: string
  port: string
  user: string
  authType: AssetAuthType
  password: string
  agentIDs: string[]
  notes: string
}

const emptyForm: AssetForm = {
  name: '',
  description: '',
  address: '',
  port: '22',
  user: '',
  authType: AssetAuthType.AuthSSHKey,
  password: '',
  agentIDs: [],
  notes: '',
}

export type AssetConfigDrawerProps = {
  open: boolean
  asset?: AssetDTO
  health?: AssetHealthDTO
  agents: BotDTO[]
  onClose: () => void
  onCreated?: (id: string) => void
  onDelete: (id: string) => void
}

const formFromAsset = (asset: AssetDTO): AssetForm => ({
  name: asset.name ?? '',
  description: asset.description ?? '',
  address: asset.server?.address ?? '',
  port: String(asset.server?.port ?? 22),
  user: asset.server?.user ?? '',
  authType: asset.server?.auth_type ?? AssetAuthType.AuthSSHKey,
  password: '',
  agentIDs: asset.agent_ids ?? [],
  notes: asset.notes_for_agents ?? '',
})

const AssetConfigDrawer: FC<AssetConfigDrawerProps> = ({ open, asset, health, agents, onClose, onCreated, onDelete }) => {
  const snackbar = useSnackbar()
  const createAsset = useCreateAsset()
  const updateAsset = useUpdateAsset()
  const linkAsset = useLinkAsset()
  const unlinkAsset = useUnlinkAsset()
  const [form, setForm] = useState<AssetForm>(emptyForm)
  const [savedForm, setSavedForm] = useState<AssetForm | null>(null)
  const [created, setCreated] = useState<AssetDTO | null>(null)
  const [enabled, setEnabled] = useState(true)
  const isEdit = Boolean(asset)

  useEffect(() => {
    if (!open) return
    const next = asset ? formFromAsset(asset) : emptyForm
    setForm(next)
    setSavedForm(asset ? next : null)
    setCreated(null)
    setEnabled(asset?.enabled !== false)
  }, [asset, open])

  const dirty = !isEdit || !savedForm || JSON.stringify(form) !== JSON.stringify(savedForm)
  const parsedPort = Number.parseInt(form.port, 10)
  const canSubmit = Boolean(form.name.trim() && form.address.trim() && form.user.trim())
    && Number.isInteger(parsedPort) && parsedPort >= 1 && parsedPort <= 65535
    && (form.authType !== AssetAuthType.AuthPassword
      || Boolean(form.password)
      || Boolean(isEdit && asset?.server?.auth_type === AssetAuthType.AuthPassword && asset.server.password_configured))
  const busy = createAsset.isPending || updateAsset.isPending || linkAsset.isPending || unlinkAsset.isPending

  const currentPublicKey = created?.server?.public_key ?? asset?.server?.public_key
  const healthStatus = useMemo(() => {
    if (!enabled) return <Chip label="Disabled" color="default" size="small" />
    if (!isEdit) return null
    const reachable = Boolean(health?.tcp_reachable && health?.ssh_reachable)
    return (
      <Chip label={reachable ? 'Connected' : 'Connection degraded'} color={reachable ? 'success' : 'warning'} size="small" />
    )
  }, [enabled, health, isEdit])

  const submit = async () => {
    if (!canSubmit) {
      snackbar.error('Name, IP address, username, authentication, and a valid port are required')
      return
    }
    try {
      if (!asset) {
        const value = await createAsset.mutateAsync({
          name: form.name.trim(),
          description: form.description.trim(),
          kind: AssetKind.KindServer,
          server: {
            address: form.address.trim(),
            port: parsedPort,
            user: form.user.trim(),
            auth_type: form.authType,
            password: form.authType === AssetAuthType.AuthPassword ? form.password : undefined,
          },
          notes_for_agents: form.notes.trim(),
        })
        setCreated(value)
        setEnabled(value.enabled !== false)
        if (value.id) onCreated?.(value.id)
        snackbar.success(`Added asset ${value.name}`)
        return
      }

      await updateAsset.mutateAsync({
        id: asset.id ?? '',
        name: form.name.trim(),
        description: form.description.trim(),
        server: {
          address: form.address.trim(),
          port: parsedPort,
          user: form.user.trim(),
          auth_type: form.authType,
          password: form.password || undefined,
        },
        notes_for_agents: form.notes.trim(),
      })

      const existing = new Set(asset.agent_ids ?? [])
      const selected = new Set(form.agentIDs)
      await Promise.all([
        ...form.agentIDs.filter((id) => !existing.has(id)).map((agentID) => linkAsset.mutateAsync({ assetID: asset.id ?? '', agentID })),
        ...(asset.agent_ids ?? []).filter((id) => !selected.has(id)).map((agentID) => unlinkAsset.mutateAsync({ assetID: asset.id ?? '', agentID })),
      ])
      setSavedForm({ ...form, password: '' })
      snackbar.success(`Saved asset ${form.name}`)
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'Could not save asset')
    }
  }

  const copyPublicKey = async () => {
    if (!currentPublicKey) return
    await navigator.clipboard.writeText(currentPublicKey)
    snackbar.success('Public key copied')
  }

  const toggleEnabled = async (nextEnabled: boolean) => {
    const current = created ?? asset
    if (!current?.id) return
    setEnabled(nextEnabled)
    try {
      const updated = await updateAsset.mutateAsync({ id: current.id, enabled: nextEnabled })
      if (created) setCreated(updated)
      snackbar.success(`${current.name ?? current.id} ${nextEnabled ? 'enabled' : 'disabled'}`)
    } catch (err: any) {
      setEnabled(!nextEnabled)
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'Could not update asset access')
    }
  }

  return (
    <HelixOrgSideDrawer
      open={open}
      onClose={onClose}
      title={created ? 'Asset created' : isEdit ? 'Edit asset' : 'New asset'}
      width={500}
    >
      <Stack spacing={1.5}>
        <HelixOrgOverviewCard
          title={created?.name || form.name || asset?.name || 'New asset'}
          id={created?.id || asset?.id || 'new asset'}
          idAction={(created?.id || asset?.id) ? <CopyButtonWithCheck text={(created?.id || asset?.id) ?? ''} /> : undefined}
          icon={<DnsOutlinedIcon sx={{ fontSize: 20 }} />}
          status={(
            <Stack direction="row" spacing={0.75} alignItems="center">
              {healthStatus ?? <Chip label="Server" size="small" sx={{ color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)', border: '1px solid rgba(255,255,255,0.22)' }} />}
              {(created || asset) && (
                <Tooltip
                  arrow
                  placement="bottom-end"
                  title={(
                    <Box sx={{ maxWidth: 280, py: 0.25 }}>
                      <Typography variant="subtitle2" color="inherit">
                        {enabled ? 'Disable asset access' : 'Enable asset access'}
                      </Typography>
                      <Typography variant="caption" color="inherit" sx={{ display: 'block', mt: 0.25, opacity: 0.85 }}>
                        {enabled
                          ? 'Blocks agents from using this asset through MCP or proxy SSH. Agent assignments stay connected.'
                          : 'Allows assigned agents to use this asset through MCP and proxy SSH again.'}
                      </Typography>
                    </Box>
                  )}
                >
                  <span>
                    <Switch
                      size="small"
                      checked={enabled}
                      disabled={updateAsset.isPending}
                      onChange={(event) => void toggleEnabled(event.target.checked)}
                      inputProps={{ 'aria-label': enabled ? 'Disable asset access' : 'Enable asset access' }}
                      sx={{
                        '& .MuiSwitch-switchBase.Mui-checked': { color: 'common.white' },
                        '& .MuiSwitch-switchBase.Mui-checked + .MuiSwitch-track': { backgroundColor: 'common.white' },
                      }}
                    />
                  </span>
                </Tooltip>
              )}
            </Stack>
          )}
        >
          {(created || asset) && <Chip label={`${form.agentIDs.length} allowed agents`} size="small" sx={{ color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)' }} />}
        </HelixOrgOverviewCard>

        {created ? (
          <>
            {currentPublicKey ? (
              <>
                <Typography variant="body2" color="text.secondary">
                  Add this Helix public key to <code>~/.ssh/authorized_keys</code> for {created.server?.user} on the server.
                </Typography>
                <TextField size="small" label="Helix public key" value={currentPublicKey} multiline minRows={4} InputProps={{ readOnly: true }} />
                <Button variant="outlined" startIcon={<ContentCopyIcon />} onClick={copyPublicKey}>Copy public key</Button>
              </>
            ) : (
              <Typography variant="body2" color="text.secondary">
                Password authentication is configured. The password is encrypted and will not be shown again.
              </Typography>
            )}
            <Button variant="contained" onClick={onClose}>Done</Button>
          </>
        ) : (
          <>
            <Typography variant="body2" color="text.secondary">
              Let AI agents troubleshoot and manage your servers at your command.
            </Typography>
            <TextField
              select size="small" label="Asset type" value={AssetKind.KindServer} fullWidth
              helperText="More first-class asset types can be added here without flattening their configuration."
            >
              <MenuItem value={AssetKind.KindServer}>Server</MenuItem>
            </TextField>
            <TextField
              size="small" label="Name" value={form.name} fullWidth required
              onChange={(e) => setForm((value) => ({ ...value, name: e.target.value.toLowerCase() }))}
              helperText="Stable SSH handle: lowercase letters, numbers, dots, dashes, and underscores."
            />
            <TextField size="small" label="Description" value={form.description} onChange={(e) => setForm((value) => ({ ...value, description: e.target.value }))} multiline minRows={2} />
            <Divider sx={{ my: 0.5 }} />
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
              <TextField size="small" label="IP address or hostname" value={form.address} onChange={(e) => setForm((value) => ({ ...value, address: e.target.value }))} required fullWidth />
              <TextField
                size="small" label="SSH port" value={form.port} required sx={{ width: { sm: 130 } }}
                onChange={(e) => setForm((value) => ({ ...value, port: e.target.value.replace(/\D/g, '') }))}
                inputProps={{ inputMode: 'numeric' }}
              />
            </Stack>
            <TextField size="small" label="Username" value={form.user} onChange={(e) => setForm((value) => ({ ...value, user: e.target.value }))} required />
            <TextField
              select size="small" label="Authentication" value={form.authType} fullWidth
              onChange={(e) => setForm((value) => ({ ...value, authType: e.target.value as AssetAuthType, password: '' }))}
            >
              <MenuItem value={AssetAuthType.AuthSSHKey}>Helix SSH key</MenuItem>
              <MenuItem value={AssetAuthType.AuthPassword}>Username and password</MenuItem>
            </TextField>
            {form.authType === AssetAuthType.AuthPassword && (
              <TextField
                size="small"
                label={asset?.server?.password_configured ? 'Replace password (optional)' : 'Password'}
                type="password"
                value={form.password}
                onChange={(e) => setForm((value) => ({ ...value, password: e.target.value }))}
                autoComplete="new-password"
                required={!asset?.server?.password_configured}
              />
            )}
            {currentPublicKey && form.authType === AssetAuthType.AuthSSHKey && (
              <>
                <TextField size="small" label="Helix public key" value={currentPublicKey} multiline minRows={4} InputProps={{ readOnly: true }} />
                <Button variant="outlined" startIcon={<ContentCopyIcon />} onClick={copyPublicKey}>Copy public key</Button>
              </>
            )}
            {asset?.server?.host_key_fingerprint && (
              <TextField size="small" label="Pinned host key" value={asset.server.host_key_fingerprint} InputProps={{ readOnly: true }} />
            )}
            {isEdit && (
              <FormControl fullWidth size="small">
                <InputLabel id="asset-agents-label">Allowed agents</InputLabel>
                <Select
                  labelId="asset-agents-label"
                  label="Allowed agents"
                  multiple
                  value={form.agentIDs}
                  onChange={(e) => setForm((value) => ({ ...value, agentIDs: typeof e.target.value === 'string' ? e.target.value.split(',') : e.target.value }))}
                  renderValue={(selected) => selected.map((id) => agents.find((agent) => agent.id === id)?.name || id).join(', ')}
                >
                  {agents.map((agent) => (
                    <MenuItem key={agent.id} value={agent.id}>
                      <Checkbox checked={form.agentIDs.includes(agent.id ?? '')} />
                      <ListItemText primary={agent.name || agent.id} secondary={agent.id} />
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
            <TextField
              size="small" label="Notes for agents" value={form.notes}
              onChange={(e) => setForm((value) => ({ ...value, notes: e.target.value }))}
              helperText="Returned to linked agents by list_assets and get_asset. Do not put credentials here."
              multiline minRows={3}
            />
            {health?.error && <Typography variant="caption" color="text.secondary">{health.error}</Typography>}
            {dirty && (
              <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ pt: 2, borderTop: '1px solid', borderColor: 'divider' }}>
                <Button color="secondary" variant="contained" onClick={submit} disabled={busy || !canSubmit}>
                  {busy ? 'Saving…' : isEdit ? 'Save' : 'Create'}
                </Button>
                <Button variant="text" onClick={onClose} disabled={busy}>Cancel</Button>
              </Stack>
            )}
            {isEdit && (
              <Box sx={{ pt: 1 }}>
                <Button color="error" onClick={() => onDelete(asset?.id ?? '')}>Delete asset</Button>
              </Box>
            )}
          </>
        )}
      </Stack>
    </HelixOrgSideDrawer>
  )
}

export default AssetConfigDrawer
