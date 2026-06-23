import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'

import { SlackLogo } from '../icons/ProviderIcons'
import SimpleTable from '../widgets/SimpleTable'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useListSlackWorkspaces,
  useListSlackApps,
  useStartSlackInstall,
  useConnectSlackWorkspace,
  useDisconnectSlackWorkspace,
} from '../../services/helixOrgService'

const SlackIntegrationsPanel: FC = () => {
  const snackbar = useSnackbar()
  const { data: workspaces = [], isLoading } = useListSlackWorkspaces()
  const { data: apps = [] } = useListSlackApps()
  const startInstall = useStartSlackInstall()
  const connectToken = useConnectSlackWorkspace()
  const disconnect = useDisconnectSlackWorkspace()

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [current, setCurrent] = useState<any | null>(null)
  const [deleting, setDeleting] = useState<any | undefined>()
  const [manualOpen, setManualOpen] = useState(false)
  const [manualToken, setManualToken] = useState('')
  const [selectedApp, setSelectedApp] = useState('')

  const appId = selectedApp || (apps.length === 1 ? apps[0].id : '')
  const chosenApp = apps.find((a: any) => a.id === appId)
  const oauthAvailable = !!chosenApp?.slack_client_id

  const handleInstall = async () => {
    try {
      const url = await startInstall.mutateAsync(appId || undefined)
      window.location.href = url
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Could not start the Slack install')
    }
  }

  const handleConnectToken = async () => {
    if (!manualToken.trim()) return
    try {
      await connectToken.mutateAsync(manualToken.trim())
      snackbar.success('Workspace connected')
      setManualToken('')
      setManualOpen(false)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Could not connect workspace')
    }
  }

  const handleDisconnect = async () => {
    if (!deleting) return
    try {
      await disconnect.mutateAsync(deleting.id)
      snackbar.success(`Disconnected ${deleting.slack_team_name || deleting.name || deleting.id}`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Disconnect failed')
    } finally {
      setDeleting(undefined)
    }
  }

  const tableData = workspaces.map((ws: any) => ({
    id: ws.id,
    _data: ws,
    workspace: (
      <Typography variant="body2" fontWeight={600}>
        {ws.slack_team_name || ws.name || ws.slack_team_id || ws.id}
      </Typography>
    ),
    team: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {ws.slack_team_id || '—'}
      </Typography>
    ),
  }))

  const getActions = (row: any) => (
    <IconButton size="small" onClick={(e) => { e.stopPropagation(); setAnchorEl(e.currentTarget); setCurrent(row._data) }}>
      <MoreVertIcon fontSize="small" />
    </IconButton>
  )

  const manualConnect = (
    <Box>
      {!manualOpen ? (
        <Button size="small" variant="text" onClick={() => setManualOpen(true)} sx={{ pl: 0 }}>
          {oauthAvailable ? 'Or connect with a bot token' : 'Connect with a bot token'}
        </Button>
      ) : (
        <Stack spacing={1}>
          <TextField
            size="small"
            fullWidth
            type="password"
            label="Bot User OAuth Token"
            placeholder="xoxb-…"
            value={manualToken}
            onChange={(e) => setManualToken(e.target.value)}
            helperText="From the Slack app's OAuth & Permissions page, after installing it into the workspace."
          />
          <Stack direction="row" spacing={1}>
            <Button variant="contained" size="small" onClick={handleConnectToken} disabled={!manualToken.trim() || connectToken.isPending}>
              Connect
            </Button>
            <Button size="small" onClick={() => { setManualOpen(false); setManualToken('') }}>Cancel</Button>
          </Stack>
        </Stack>
      )}
    </Box>
  )

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
        <SlackLogo sx={{ fontSize: 20, color: 'primary.main' }} />
        <Typography variant="h6">Slack</Typography>
      </Stack>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Connect a Slack workspace, then bind topics to channels. Workers reply as their persona;
        route to a specific Worker with a filter (e.g. <code>!qa-bot</code>).
      </Typography>

      <Stack spacing={2}>
        <Box sx={{ p: 2, borderRadius: 1, bgcolor: 'action.hover' }}>
          {apps.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No Slack app has been configured by an administrator yet.
            </Typography>
          ) : (
            <Stack spacing={1.5}>
              {apps.length > 1 && (
                <FormControl size="small" sx={{ maxWidth: 280 }}>
                  <InputLabel>Slack app</InputLabel>
                  <Select label="Slack app" value={appId} onChange={(e) => setSelectedApp(e.target.value)}>
                    {apps.map((a: any) => (
                      <MenuItem key={a.id} value={a.id}>
                        {a.name || a.id}{a.slack_ingress_mode ? ` · ${a.slack_ingress_mode}` : ''}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}

              {apps.length > 1 && !chosenApp ? (
                <Typography variant="body2" color="text.secondary">Choose a Slack app to connect a workspace.</Typography>
              ) : oauthAvailable ? (
                <Stack spacing={1.5} alignItems="flex-start">
                  <Button variant="contained" startIcon={<SlackLogo sx={{ fontSize: 18 }} />} onClick={handleInstall} disabled={startInstall.isPending}>
                    Add to Slack
                  </Button>
                  {manualConnect}
                </Stack>
              ) : (
                <Stack spacing={1}>
                  <Typography variant="body2" color="text.secondary">
                    This app connects by bot token. Install it into your workspace in Slack, then paste the bot token here.
                  </Typography>
                  {manualConnect}
                </Stack>
              )}
            </Stack>
          )}
        </Box>

        <Box>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>Connected workspaces</Typography>
          {!isLoading && workspaces.length === 0 ? (
            <Typography variant="body2" color="text.secondary">None yet — add one above.</Typography>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'workspace', title: 'Workspace' },
                { name: 'team', title: 'Team ID' },
              ]}
              data={tableData}
              getActions={getActions}
            />
          )}
        </Box>
      </Stack>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => { setAnchorEl(null); setCurrent(null) }}>
        <MenuItem onClick={() => { setDeleting(current); setAnchorEl(null); setCurrent(null) }}>
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Disconnect
        </MenuItem>
      </Menu>

      {deleting && (
        <DeleteConfirmWindow
          title="Slack workspace"
          submitTitle="Disconnect"
          onCancel={() => setDeleting(undefined)}
          onSubmit={handleDisconnect}
        >
          <Typography variant="body1">
            Disconnect <b>{deleting.slack_team_name || deleting.slack_team_id || deleting.id}</b>?
            Topics bound to it stop sending and receiving. The Helix bot stays in your
            workspace until you remove it from Slack.
          </Typography>
        </DeleteConfirmWindow>
      )}
    </Paper>
  )
}

export default SlackIntegrationsPanel
