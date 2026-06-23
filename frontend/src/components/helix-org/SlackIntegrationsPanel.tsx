// SlackIntegrationsPanel lets an org admin install the deployment-wide
// Helix Slack app into their own workspace(s) and manage the resulting
// connections. Mirrors GitHubAppPanel's place on the org Settings page.
//
// Install is a top-level OAuth redirect: we ask the backend (with the
// user's token) for the authorize URL, then send the browser to Slack.
// Slack redirects back to /api/v1/slack/oauth/callback, which persists
// the org-scoped slack_workspace connection and returns the user here.

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import AddIcon from '@mui/icons-material/Add'

import SimpleTable from '../widgets/SimpleTable'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useListSlackWorkspaces,
  useStartSlackInstall,
  useConnectSlackWorkspace,
  useDisconnectSlackWorkspace,
} from '../../services/helixOrgService'

const SlackIntegrationsPanel: FC = () => {
  const snackbar = useSnackbar()
  const { data: workspaces = [], isLoading } = useListSlackWorkspaces()
  const startInstall = useStartSlackInstall()
  const connectToken = useConnectSlackWorkspace()
  const disconnect = useDisconnectSlackWorkspace()

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [current, setCurrent] = useState<any | null>(null)
  const [deleting, setDeleting] = useState<any | undefined>()
  const [manualOpen, setManualOpen] = useState(false)
  const [manualToken, setManualToken] = useState('')

  const handleInstall = async () => {
    try {
      const url = await startInstall.mutateAsync()
      window.location.href = url
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Slack is not configured by the administrator yet')
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
      <Typography variant="body2" fontWeight="medium">
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
    <IconButton
      size="small"
      onClick={(e) => { e.stopPropagation(); setAnchorEl(e.currentTarget); setCurrent(row._data) }}
    >
      <MoreVertIcon />
    </IconButton>
  )

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
        <Box>
          <Typography variant="subtitle1" fontWeight="bold">Slack</Typography>
          <Typography variant="body2" color="text.secondary">
            Install the Helix Slack app into your workspace, then bind topics to channels.
            Workers reply as their persona; route to a specific Worker with a filter (e.g. !qa-bot).
          </Typography>
        </Box>
        <Button
          variant="contained"
          size="small"
          startIcon={<AddIcon />}
          onClick={handleInstall}
          disabled={startInstall.isPending}
        >
          Install to Slack
        </Button>
      </Box>

      {/* Manual / Socket Mode connect: paste a bot token. On-prem (Socket
          Mode) has no OAuth, so the workspace is connected this way. */}
      <Box sx={{ mt: 1 }}>
        {!manualOpen ? (
          <Button size="small" variant="text" onClick={() => setManualOpen(true)} sx={{ px: 0 }}>
            Or connect with a bot token (Socket Mode / on-prem)
          </Button>
        ) : (
          <Stack direction="row" spacing={1} alignItems="flex-start" sx={{ mt: 1 }}>
            <TextField
              size="small"
              fullWidth
              type="password"
              label="Bot User OAuth Token"
              placeholder="xoxb-…"
              value={manualToken}
              onChange={(e) => setManualToken(e.target.value)}
              helperText="From your Slack app's OAuth & Permissions page, after installing it into the workspace."
            />
            <Button variant="contained" size="small" sx={{ mt: 0.5 }} onClick={handleConnectToken}
              disabled={!manualToken.trim() || connectToken.isPending}>
              Connect
            </Button>
            <Button size="small" sx={{ mt: 0.5 }} onClick={() => { setManualOpen(false); setManualToken('') }}>
              Cancel
            </Button>
          </Stack>
        )}
      </Box>

      {!isLoading && workspaces.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          No workspaces connected yet.
        </Typography>
      ) : (
        <Box sx={{ mt: 2 }}>
          <SimpleTable
            authenticated={true}
            fields={[
              { name: 'workspace', title: 'Workspace' },
              { name: 'team', title: 'Team ID' },
            ]}
            data={tableData}
            getActions={getActions}
          />
        </Box>
      )}

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => { setAnchorEl(null); setCurrent(null) }}>
        <MenuItem
          onClick={() => { setDeleting(current); setAnchorEl(null); setCurrent(null) }}
        >
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
