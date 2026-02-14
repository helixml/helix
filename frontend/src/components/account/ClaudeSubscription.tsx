import React, { FC, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import TextField from '@mui/material/TextField'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import DeleteIcon from '@mui/icons-material/Delete'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import DialogContentText from '@mui/material/DialogContentText'
import Alert from '@mui/material/Alert'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'

interface ClaudeSubscriptionData {
  id: string
  created: string
  name: string
  subscription_type: string
  rate_limit_tier: string
  status: string
  access_token_expires_at: string
  owner_type: string
  owner_id: string
}

const ClaudeSubscription: FC = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const queryClient = useQueryClient()

  const [connectDialogOpen, setConnectDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string>('')
  const [credentialsText, setCredentialsText] = useState('')
  const [subscriptionName, setSubscriptionName] = useState('My Claude Subscription')

  // Fetch existing subscriptions
  const { data: subscriptions, isLoading } = useQuery({
    queryKey: ['claude-subscriptions'],
    queryFn: async () => {
      const result = await api.get<ClaudeSubscriptionData[]>('/api/v1/claude-subscriptions', {})
      return result || []
    },
  })

  // Create subscription mutation
  const createMutation = useMutation({
    mutationFn: async (data: { name: string; credentials: any }) => {
      return api.post('/api/v1/claude-subscriptions', data, {}, {
        snackbar: true,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      setConnectDialogOpen(false)
      setCredentialsText('')
      snackbar.success('Claude subscription connected')
    },
  })

  // Delete subscription mutation
  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      return api.delete(`/api/v1/claude-subscriptions/${id}`, {}, {
        snackbar: true,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      setDeleteDialogOpen(false)
      setDeleteTarget('')
      snackbar.success('Claude subscription disconnected')
    },
  })

  const handleConnect = useCallback(() => {
    // Parse the credentials JSON
    let parsed: any
    try {
      parsed = JSON.parse(credentialsText)
    } catch {
      snackbar.error('Invalid JSON. Please paste the full contents of your .credentials.json file.')
      return
    }

    // Accept either the full file format or just the claudeAiOauth object
    let creds = parsed.claudeAiOauth || parsed
    if (!creds.accessToken || !creds.refreshToken) {
      snackbar.error('Missing accessToken or refreshToken. Please paste the full contents of ~/.claude/.credentials.json')
      return
    }

    createMutation.mutate({
      name: subscriptionName,
      credentials: {
        claudeAiOauth: creds,
      },
    })
  }, [credentialsText, subscriptionName, createMutation])

  const handleDeleteClick = useCallback((id: string) => {
    setDeleteTarget(id)
    setDeleteDialogOpen(true)
  }, [])

  const handleConfirmDelete = useCallback(() => {
    if (deleteTarget) {
      deleteMutation.mutate(deleteTarget)
    }
  }, [deleteTarget, deleteMutation])

  const hasSubscription = subscriptions && subscriptions.length > 0

  return (
    <>
      <Grid container spacing={2} sx={{ mt: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
        <Grid item xs={12}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Box>
              <Typography variant="h6">Claude Code Subscription</Typography>
              <Typography variant="body2" color="text.secondary">
                Connect your Claude subscription to use Claude Code as the coding agent in Helix desktop sessions.
              </Typography>
            </Box>
            {!hasSubscription && (
              <Button
                variant="contained"
                color="secondary"
                onClick={() => setConnectDialogOpen(true)}
              >
                Connect
              </Button>
            )}
          </Box>

          {isLoading ? (
            <Typography variant="body2" color="text.secondary">Loading...</Typography>
          ) : hasSubscription ? (
            subscriptions.map((sub) => (
              <Box
                key={sub.id}
                sx={{
                  p: 2,
                  borderRadius: 1,
                  border: '1px solid',
                  borderColor: 'divider',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  mb: 1,
                }}
              >
                <Box>
                  <Typography variant="subtitle1">{sub.name || 'Claude Subscription'}</Typography>
                  <Box sx={{ display: 'flex', gap: 1, mt: 0.5, alignItems: 'center' }}>
                    <Chip
                      label={sub.status === 'active' ? 'Connected' : sub.status}
                      color={sub.status === 'active' ? 'success' : 'warning'}
                      size="small"
                    />
                    {sub.subscription_type && (
                      <Chip label={sub.subscription_type} size="small" variant="outlined" />
                    )}
                    {sub.owner_type === 'org' && (
                      <Chip label="Organization" size="small" variant="outlined" />
                    )}
                  </Box>
                </Box>
                <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                  <Button
                    variant="outlined"
                    color="secondary"
                    size="small"
                    onClick={() => setConnectDialogOpen(true)}
                  >
                    Update
                  </Button>
                  <IconButton
                    color="error"
                    size="small"
                    onClick={() => handleDeleteClick(sub.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                </Box>
              </Box>
            ))
          ) : (
            <Box sx={{ p: 2, borderRadius: 1, border: '1px dashed', borderColor: 'divider', textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                No Claude subscription connected. Connect your subscription to use Claude Code in desktop sessions.
              </Typography>
            </Box>
          )}
        </Grid>
      </Grid>

      {/* Connect Dialog */}
      <Dialog open={connectDialogOpen} onClose={() => setConnectDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Connect Claude Subscription</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            To connect your Claude subscription, paste the contents of your local Claude credentials file.
            This file is located at <code>~/.claude/.credentials.json</code> on your computer.
          </Alert>

          <TextField
            fullWidth
            label="Subscription Name"
            value={subscriptionName}
            onChange={(e) => setSubscriptionName(e.target.value)}
            sx={{ mb: 2, mt: 1 }}
          />

          <TextField
            fullWidth
            label="Credentials JSON"
            multiline
            rows={8}
            value={credentialsText}
            onChange={(e) => setCredentialsText(e.target.value)}
            placeholder='Paste the contents of ~/.claude/.credentials.json here'
            sx={{ fontFamily: 'monospace' }}
          />

          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
            Your credentials are encrypted at rest. Claude Code handles token refresh natively inside desktop containers.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConnectDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleConnect}
            variant="contained"
            color="secondary"
            disabled={!credentialsText || createMutation.isPending}
          >
            {createMutation.isPending ? 'Connecting...' : 'Connect'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Disconnect Claude Subscription</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to disconnect this Claude subscription? Desktop sessions will no longer be able to use Claude Code.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleConfirmDelete}
            color="error"
            variant="contained"
            disabled={deleteMutation.isPending}
          >
            Disconnect
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default ClaudeSubscription
