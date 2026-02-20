import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
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
import CircularProgress from '@mui/material/CircularProgress'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'
import useAccount from '../../hooks/useAccount'
import ExternalAgentDesktopViewer, { useSandboxState } from '../external-agent/ExternalAgentDesktopViewer'
import { getTokenExpiryStatus } from './claudeSubscriptionUtils'

interface ClaudeSubscriptionData {
  id: string
  created: string
  name: string
  subscription_type: string
  rate_limit_tier: string
  status: string
  access_token_expires_at: string
  last_refreshed_at?: string
  owner_type: string
  owner_id: string
}

const ClaudeSubscription: FC = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const queryClient = useQueryClient()
  const account = useAccount()

  const organizations = account.organizationTools.organizations || []

  const [connectDialogOpen, setConnectDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string>('')
  const [credentialsText, setCredentialsText] = useState('')
  const [subscriptionName, setSubscriptionName] = useState('My Claude Subscription')
  const [ownerType, setOwnerType] = useState<'user' | 'org'>('user')
  const [selectedOrgId, setSelectedOrgId] = useState('')

  // Interactive login state
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [loginSessionId, setLoginSessionId] = useState<string>('')
  const [loginStarting, setLoginStarting] = useState(false)
  const [loginCommandSent, setLoginCommandSent] = useState(false)
  const [loginPolling, setLoginPolling] = useState(false)
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

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
      ...(ownerType === 'org' && selectedOrgId ? { owner_type: 'org', owner_id: selectedOrgId } : {}),
    })
  }, [credentialsText, subscriptionName, ownerType, selectedOrgId])

  const handleDeleteClick = useCallback((id: string) => {
    setDeleteTarget(id)
    setDeleteDialogOpen(true)
  }, [])

  const handleConfirmDelete = useCallback(() => {
    if (deleteTarget) {
      deleteMutation.mutate(deleteTarget)
    }
  }, [deleteTarget])

  // Start interactive login flow
  const handleStartLogin = useCallback(async () => {
    setLoginStarting(true)
    try {
      const result = await api.post<{ session_id: string }>('/api/v1/claude-subscriptions/start-login', {})
      if (result && result.session_id) {
        setLoginSessionId(result.session_id)
        setLoginDialogOpen(true)
        setLoginCommandSent(false)
        setLoginPolling(false)
      }
    } catch (err: any) {
      snackbar.error('Failed to start login session: ' + (err?.message || 'unknown error'))
    } finally {
      setLoginStarting(false)
    }
  }, [])

  // Stop login session and clean up
  const stopLoginSession = useCallback(async (sessionId: string) => {
    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current)
      pollIntervalRef.current = null
    }
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1SessionsStopExternalAgentDelete(sessionId)
    } catch {
      // Ignore errors when stopping
    }
  }, [])

  const handleCloseLoginDialog = useCallback(() => {
    if (loginSessionId) {
      stopLoginSession(loginSessionId)
    }
    setLoginDialogOpen(false)
    setLoginSessionId('')
    setLoginCommandSent(false)
    setLoginPolling(false)
  }, [loginSessionId])

  // Clean up polling on unmount
  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
      }
    }
  }, [])

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
              <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                {organizations.length > 0 && (
                  <FormControl size="small" sx={{ minWidth: 200 }}>
                    <InputLabel>Subscription Owner</InputLabel>
                    <Select
                      value={ownerType === 'org' ? selectedOrgId : 'personal'}
                      label="Subscription Owner"
                      onChange={(e) => {
                        const val = e.target.value
                        if (val === 'personal') {
                          setOwnerType('user')
                          setSelectedOrgId('')
                        } else {
                          setOwnerType('org')
                          setSelectedOrgId(val)
                        }
                      }}
                    >
                      <MenuItem value="personal">Personal (just me)</MenuItem>
                      {organizations.map((org) => (
                        <MenuItem key={org.id} value={org.id}>
                          {org.display_name || org.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                )}
                <Button
                  variant="contained"
                  color="secondary"
                  onClick={handleStartLogin}
                  disabled={loginStarting}
                >
                  {loginStarting ? 'Starting...' : 'Login with Browser'}
                </Button>
                <Button
                  variant="outlined"
                  color="secondary"
                  onClick={() => setConnectDialogOpen(true)}
                >
                  Paste Credentials
                </Button>
              </Box>
            )}
          </Box>

          {isLoading ? (
            <Typography variant="body2" color="text.secondary">Loading...</Typography>
          ) : hasSubscription ? (
            subscriptions.map((sub) => {
              const expiry = getTokenExpiryStatus(sub.access_token_expires_at)
              const isExpired = expiry?.isExpired ?? false
              return (
                <Box
                  key={sub.id}
                  sx={{
                    p: 2,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: isExpired ? 'error.main' : expiry?.isExpiringSoon ? 'warning.main' : 'divider',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    mb: 1,
                  }}
                >
                  <Box>
                    <Typography variant="subtitle1">{sub.name || 'Claude Subscription'}</Typography>
                    <Box sx={{ display: 'flex', gap: 1, mt: 0.5, alignItems: 'center', flexWrap: 'wrap' }}>
                      {isExpired ? (
                        <Chip
                          icon={<ErrorOutlineIcon />}
                          label="Token Expired"
                          color="error"
                          size="small"
                        />
                      ) : (
                        <Chip
                          label={sub.status === 'active' ? 'Connected' : sub.status}
                          color={sub.status === 'active' ? 'success' : 'warning'}
                          size="small"
                        />
                      )}
                      {sub.subscription_type && (
                        <Chip label={sub.subscription_type} size="small" variant="outlined" />
                      )}
                      {sub.owner_type === 'org' && (
                        <Chip label="Organization" size="small" variant="outlined" />
                      )}
                      {expiry && (
                        <Typography
                          variant="caption"
                          color={`${expiry.color}.main`}
                          sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                        >
                          {expiry.isExpiringSoon && !isExpired && <WarningAmberIcon sx={{ fontSize: 14 }} />}
                          {expiry.label}
                        </Typography>
                      )}
                    </Box>
                    {isExpired && (
                      <Alert severity="warning" sx={{ mt: 1, py: 0 }} icon={false}>
                        <Typography variant="caption">
                          Token has expired. Re-login to refresh your credentials for new sessions.
                          Active sessions refresh tokens automatically.
                        </Typography>
                      </Alert>
                    )}
                  </Box>
                  <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                    <Button
                      variant={isExpired ? 'contained' : 'outlined'}
                      color={isExpired ? 'warning' : 'secondary'}
                      size="small"
                      onClick={handleStartLogin}
                      disabled={loginStarting}
                    >
                      {loginStarting ? 'Starting...' : isExpired ? 'Re-authenticate' : 'Re-login'}
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
              )
            })
          ) : (
            <Box sx={{ p: 2, borderRadius: 1, border: '1px dashed', borderColor: 'divider', textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                No Claude subscription connected. Click "Login with Browser" to sign in with your Claude account.
              </Typography>
            </Box>
          )}
        </Grid>
      </Grid>

      {/* Interactive Login Dialog */}
      {loginDialogOpen && loginSessionId && (
        <ClaudeLoginDialog
          sessionId={loginSessionId}
          open={loginDialogOpen}
          onClose={handleCloseLoginDialog}
          loginCommandSent={loginCommandSent}
          setLoginCommandSent={setLoginCommandSent}
          loginPolling={loginPolling}
          setLoginPolling={setLoginPolling}
          pollIntervalRef={pollIntervalRef}
          subscriptionName={subscriptionName}
          ownerType={ownerType}
          selectedOrgId={selectedOrgId}
          onCredentialsCaptured={() => {
            queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
            snackbar.success('Claude subscription connected via browser login')
            handleCloseLoginDialog()
          }}
        />
      )}

      {/* Paste Credentials Dialog */}
      <Dialog open={connectDialogOpen} onClose={() => setConnectDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Paste Claude Credentials</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            Paste the contents of your local Claude credentials file.
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

// Separate component for the login dialog to properly use useSandboxState
interface ClaudeLoginDialogProps {
  sessionId: string
  open: boolean
  onClose: () => void
  loginCommandSent: boolean
  setLoginCommandSent: (v: boolean) => void
  loginPolling: boolean
  setLoginPolling: (v: boolean) => void
  pollIntervalRef: React.MutableRefObject<ReturnType<typeof setInterval> | null>
  subscriptionName: string
  ownerType: 'user' | 'org'
  selectedOrgId: string
  onCredentialsCaptured: () => void
}

const ClaudeLoginDialog: FC<ClaudeLoginDialogProps> = ({
  sessionId,
  open,
  onClose,
  loginCommandSent,
  setLoginCommandSent,
  loginPolling,
  setLoginPolling,
  pollIntervalRef,
  subscriptionName,
  ownerType,
  selectedOrgId,
  onCredentialsCaptured,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const { isRunning } = useSandboxState(sessionId)

  // Once the desktop is running, send the `claude auth login` command
  useEffect(() => {
    if (!isRunning || loginCommandSent) return

    const sendLoginCommand = async () => {
      try {
        const apiClient = api.getApiClient()
        await apiClient.v1ExternalAgentsExecCreate(sessionId, {
          command: ['claude', 'auth', 'login'],
          background: true,
          env: {
            DISPLAY: ':0',
          },
        })
        setLoginCommandSent(true)
      } catch (err: any) {
        console.error('Failed to send claude auth login command:', err)
      }
    }

    // Small delay to let GNOME initialize
    const timeout = setTimeout(sendLoginCommand, 3000)
    return () => clearTimeout(timeout)
  }, [isRunning, loginCommandSent, sessionId])

  // Once login command is sent, start polling for credentials.
  // Use a ref guard instead of loginPolling state in deps to prevent the effect cleanup
  // from killing the interval on re-render (state change -> re-render -> cleanup -> no interval).
  const pollingStartedRef = useRef(false)
  useEffect(() => {
    if (!loginCommandSent || pollingStartedRef.current) return

    pollingStartedRef.current = true

    const pollForCredentials = async () => {
      try {
        const result = await api.get<{ found: boolean; credentials: string }>(
          `/api/v1/claude-subscriptions/poll-login/${sessionId}`,
          {}
        )
        if (result && result.found && result.credentials) {
          let parsed: any
          try {
            parsed = JSON.parse(result.credentials)
          } catch {
            return
          }

          const creds = parsed.claudeAiOauth || parsed
          if (!creds.accessToken || !creds.refreshToken) return

          await api.post('/api/v1/claude-subscriptions', {
            name: subscriptionName,
            credentials: {
              claudeAiOauth: creds,
            },
            ...(ownerType === 'org' && selectedOrgId ? { owner_type: 'org', owner_id: selectedOrgId } : {}),
          })

          onCredentialsCaptured()
        }
      } catch {
        // Ignore polling errors
      }
    }

    pollIntervalRef.current = setInterval(pollForCredentials, 3000)

    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
        pollIntervalRef.current = null
      }
      pollingStartedRef.current = false
    }
  }, [loginCommandSent, sessionId, subscriptionName, ownerType, selectedOrgId])

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: { height: '95vh', maxHeight: '95vh' },
      }}
    >
      <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Box>
          <Typography variant="h6">Sign in to Claude</Typography>
          <Typography variant="body2" color="text.secondary">
            Complete the login in the browser below. Your credentials will automatically be reused in desktop sessions configured to use Claude Code.
          </Typography>
        </Box>
        {loginCommandSent && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <CircularProgress size={16} />
            <Typography variant="body2" color="text.secondary">
              Waiting for login...
            </Typography>
          </Box>
        )}
      </DialogTitle>
      <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {isRunning && loginCommandSent && (
          <Alert severity="info" sx={{ mx: 2, mt: 1, flexShrink: 0 }}>
            Enter your email address in the browser below. Claude will email you a link — click it to get a code, then paste the code back here to authenticate.
          </Alert>
        )}
        {!isRunning ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', flex: 1, gap: 2 }}>
            <CircularProgress />
            <Typography variant="body2" color="text.secondary">
              Starting desktop session...
            </Typography>
          </Box>
        ) : (
          <Box sx={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
            <ExternalAgentDesktopViewer
              sessionId={sessionId}
              mode="stream"
            />
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  )
}

export default ClaudeSubscription
