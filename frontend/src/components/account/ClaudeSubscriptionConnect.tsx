import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogActions from '@mui/material/DialogActions'
import CircularProgress from '@mui/material/CircularProgress'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useSandboxState } from '../external-agent/ExternalAgentDesktopViewer'
import { getTokenExpiryStatus, openExternalUrl } from './claudeSubscriptionUtils'

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

// Shared hook for querying Claude subscription status
export function useClaudeSubscriptions() {
  const api = useApi()
  return useQuery({
    queryKey: ['claude-subscriptions'],
    queryFn: async () => {
      const result = await api.get<ClaudeSubscriptionData[]>('/api/v1/claude-subscriptions', {})
      return result || []
    },
  })
}

interface ClaudeSubscriptionConnectProps {
  // Render as a button (for provider grids) or inline (for settings pages)
  variant?: 'button' | 'inline'
  // Called after successful connection
  onConnected?: () => void
  // When provided, creates an org-level subscription instead of user-level
  orgId?: string
}

// Reusable component for connecting a Claude subscription via browser login.
// Used in: Onboarding, Providers page, Account settings.
const ClaudeSubscriptionConnect: FC<ClaudeSubscriptionConnectProps> = ({
  variant = 'button',
  onConnected,
  orgId,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()

  const { data: subscriptions } = useClaudeSubscriptions()
  const hasSubscription = subscriptions && subscriptions.length > 0

  // Disconnect state
  const [disconnectDialogOpen, setDisconnectDialogOpen] = useState(false)
  const disconnectMutation = useMutation({
    mutationFn: async (id: string) => {
      return api.delete(`/api/v1/claude-subscriptions/${id}`, {}, {
        snackbar: true,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      setDisconnectDialogOpen(false)
      snackbar.success('Claude subscription disconnected')
    },
  })

  // Interactive login state
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [loginSessionId, setLoginSessionId] = useState<string>('')
  const [loginStarting, setLoginStarting] = useState(false)
  const [loginCommandSent, setLoginCommandSent] = useState(false)
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Start interactive login flow
  const handleStartLogin = useCallback(async () => {
    setLoginStarting(true)
    try {
      const result = await api.post<{}, { session_id: string }>('/api/v1/claude-subscriptions/start-login', {})
      if (result && result.session_id) {
        setLoginSessionId(result.session_id)
        setLoginDialogOpen(true)
        setLoginCommandSent(false)
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
  }, [loginSessionId])

  // Clean up polling on unmount
  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
      }
    }
  }, [])

  const firstSub = subscriptions?.[0]
  const expiry = firstSub ? getTokenExpiryStatus(firstSub.access_token_expires_at) : null
  const isExpired = expiry?.isExpired ?? false

  if (variant === 'button') {
    return (
      <>
        {hasSubscription ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.5 }}>
            {isExpired ? (
              <Button
                size="small"
                variant="contained"
                color="warning"
                onClick={handleStartLogin}
                disabled={loginStarting}
                startIcon={loginStarting ? <CircularProgress size={14} /> : <ErrorOutlineIcon />}
              >
                {loginStarting ? 'Starting...' : 'Re-authenticate'}
              </Button>
            ) : (
              <Button
                size="small"
                variant="outlined"
                color="error"
                onClick={() => setDisconnectDialogOpen(true)}
              >
                Disconnect
              </Button>
            )}
            {expiry && (
              <Typography variant="caption" color={`${expiry.color}.main`} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {expiry.isExpiringSoon && !isExpired && <WarningAmberIcon sx={{ fontSize: 12 }} />}
                {isExpired && <ErrorOutlineIcon sx={{ fontSize: 12 }} />}
                {expiry.label}
              </Typography>
            )}
          </Box>
        ) : (
          <Button
            size="small"
            variant="text"
            color="secondary"
            onClick={handleStartLogin}
            disabled={loginStarting}
          >
            {loginStarting ? <><CircularProgress size={14} sx={{ mr: 0.5 }} /> Starting...</> : 'Connect'}
          </Button>
        )}

        <Dialog open={disconnectDialogOpen} onClose={() => setDisconnectDialogOpen(false)}>
          <DialogTitle>Disconnect Claude Subscription</DialogTitle>
          <DialogContent>
            <DialogContentText>
              Are you sure you want to disconnect your Claude subscription?
            </DialogContentText>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDisconnectDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={() => {
                if (subscriptions?.[0]?.id) {
                  disconnectMutation.mutate(subscriptions[0].id)
                }
              }}
              color="error"
              variant="contained"
              disabled={disconnectMutation.isPending}
            >
              {disconnectMutation.isPending ? 'Disconnecting...' : 'Disconnect'}
            </Button>
          </DialogActions>
        </Dialog>

        {loginDialogOpen && loginSessionId && (
          <ClaudeLoginDialogInner
            sessionId={loginSessionId}
            open={loginDialogOpen}
            onClose={handleCloseLoginDialog}
            loginCommandSent={loginCommandSent}
            setLoginCommandSent={setLoginCommandSent}
            pollIntervalRef={pollIntervalRef}
            orgId={orgId}
            onCredentialsCaptured={() => {
              queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
              snackbar.success('Claude subscription connected')
              handleCloseLoginDialog()
              onConnected?.()
            }}
          />
        )}
      </>
    )
  }

  // inline variant - used in settings
  return (
    <>
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
        {hasSubscription ? (
          <>
            {isExpired ? (
              <ErrorOutlineIcon color="error" fontSize="small" />
            ) : (
              <CheckCircleIcon color="success" fontSize="small" />
            )}
            <Typography variant="body2" color={isExpired ? 'error.main' : 'success.main'}>
              {isExpired ? 'Claude token expired' : 'Claude subscription connected'}
            </Typography>
            {expiry && (
              <Typography variant="caption" color={`${expiry.color}.main`} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                ({expiry.label})
              </Typography>
            )}
            <Button
              size="small"
              variant={isExpired ? 'contained' : 'text'}
              color={isExpired ? 'warning' : 'primary'}
              onClick={handleStartLogin}
              disabled={loginStarting}
            >
              {loginStarting ? <><CircularProgress size={14} sx={{ mr: 0.5 }} /> Starting...</> : isExpired ? 'Re-authenticate' : 'Re-login'}
            </Button>
          </>
        ) : (
          <Button
            variant="contained"
            color="secondary"
            onClick={handleStartLogin}
            disabled={loginStarting}
          >
            {loginStarting ? <><CircularProgress size={14} sx={{ mr: 0.5 }} /> Starting...</> : 'Login with Browser'}
          </Button>
        )}
      </Box>

      {loginDialogOpen && loginSessionId && (
        <ClaudeLoginDialogInner
          sessionId={loginSessionId}
          open={loginDialogOpen}
          onClose={handleCloseLoginDialog}
          loginCommandSent={loginCommandSent}
          setLoginCommandSent={setLoginCommandSent}
          pollIntervalRef={pollIntervalRef}
          orgId={orgId}
          onCredentialsCaptured={() => {
            queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
            snackbar.success('Claude subscription connected')
            handleCloseLoginDialog()
            onConnected?.()
          }}
        />
      )}
    </>
  )
}

// Inner dialog component - needs to be separate to use useSandboxState hook
interface ClaudeLoginDialogInnerProps {
  sessionId: string
  open: boolean
  onClose: () => void
  loginCommandSent: boolean
  setLoginCommandSent: (v: boolean) => void
  pollIntervalRef: React.MutableRefObject<ReturnType<typeof setInterval> | null>
  onCredentialsCaptured: () => void
  orgId?: string
}

const POLL_TIMEOUT_MS = 5 * 60 * 1000 // 5 minutes

const ClaudeLoginDialogInner: FC<ClaudeLoginDialogInnerProps> = ({
  sessionId,
  open,
  onClose,
  loginCommandSent,
  setLoginCommandSent,
  pollIntervalRef,
  onCredentialsCaptured,
  orgId,
}) => {
  const api = useApi()
  const { isRunning } = useSandboxState(sessionId)
  const [authUrl, setAuthUrl] = useState<string | null>(null)
  const [loginError, setLoginError] = useState<string | null>(null)
  const [authCode, setAuthCode] = useState('')
  const [codeSubmitting, setCodeSubmitting] = useState(false)
  const [codeSubmitted, setCodeSubmitted] = useState(false)

  // Once the desktop is running, send the `claude auth login` command
  useEffect(() => {
    if (!isRunning || loginCommandSent) return

    const sendLoginCommand = async () => {
      try {
        const apiClient = api.getApiClient()
        // The wrapper script handles npm install (with retry for network readiness),
        // sets BROWSER to the capture script, and runs claude auth login with stdout
        // redirected to /tmp/claude-auth-stdout.txt for URL parsing.
        await apiClient.v1ExternalAgentsExecCreate(sessionId, {
          command: ['helix-claude-auth-wrapper'],
          background: true,
          env: {},
        })
        setLoginCommandSent(true)
      } catch (err: any) {
        console.error('Failed to send claude auth login command:', err)
        setLoginError(err?.message || 'Failed to start Claude login. Please try again.')
      }
    }

    // Small delay to let the container initialize
    const timeout = setTimeout(sendLoginCommand, 3000)
    return () => clearTimeout(timeout)
  }, [isRunning, loginCommandSent, sessionId])

  // Submit the auth code to the container's named pipe
  const handleSubmitCode = useCallback(async () => {
    if (!authCode.trim()) return
    setCodeSubmitting(true)
    try {
      const apiClient = api.getApiClient()
      // Write the code to the named pipe that the wrapper script reads from.
      // claude auth login is waiting for this on stdin (via the fifo).
      await apiClient.v1ExternalAgentsExecCreate(sessionId, {
        command: ['helix-claude-auth-submit', authCode.trim()],
        background: false,
        env: {},
      })
      setCodeSubmitted(true)
    } catch (err: any) {
      console.error('Failed to submit auth code:', err)
      setLoginError('Failed to submit authentication code. Please try again.')
    } finally {
      setCodeSubmitting(false)
    }
  }, [authCode, sessionId])

  // Once login command is sent, start polling for credentials (and OAuth URL).
  // Use a ref guard instead of state in deps to prevent the effect cleanup
  // from killing the interval on re-render (state change -> re-render -> cleanup -> no interval).
  const pollingStartedRef = useRef(false)
  useEffect(() => {
    if (!loginCommandSent || pollingStartedRef.current) return

    pollingStartedRef.current = true
    const pollStartTime = Date.now()

    const pollForCredentials = async () => {
      // Timeout after 5 minutes to avoid spinning forever
      if (Date.now() - pollStartTime > POLL_TIMEOUT_MS) {
        if (pollIntervalRef.current) {
          clearInterval(pollIntervalRef.current)
          pollIntervalRef.current = null
        }
        setLoginError('Authentication timed out. Please try again.')
        return
      }

      try {
        const result = await api.get<{ found: boolean; credentials: string; url?: string }>(
          `/api/v1/claude-subscriptions/poll-login/${sessionId}`,
          {}
        )

        // Check for credentials first
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
            name: 'My Claude Subscription',
            credentials: {
              claudeAiOauth: creds,
            },
            ...(orgId ? { owner_type: 'org', owner_id: orgId } : {}),
          })

          onCredentialsCaptured()
          return
        }

        // Set the OAuth URL so the UI can show the sign-in button.
        // We don't call window.open() here because this runs inside a
        // setInterval callback — browsers block popups without a user gesture.
        if (result?.url && !authUrl) {
          setAuthUrl(result.url)
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
  }, [loginCommandSent, sessionId, orgId])

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        <Typography variant="h6">Sign in to Claude</Typography>
        <Typography variant="body2" color="text.secondary">
          Your credentials will automatically be reused in desktop sessions configured to use Claude Code.
        </Typography>
      </DialogTitle>
      <DialogContent>
        {loginError ? (
          <Alert severity="error" sx={{ my: 2 }}>
            {loginError}
          </Alert>
        ) : !isRunning || !loginCommandSent ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4, gap: 2 }}>
            <CircularProgress />
            <Typography variant="body2" color="text.secondary">
              Preparing authentication...
            </Typography>
          </Box>
        ) : !authUrl ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4, gap: 2 }}>
            <CircularProgress />
            <Typography variant="body2" color="text.secondary">
              Waiting for Claude login page...
            </Typography>
          </Box>
        ) : !codeSubmitted ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography variant="body2">
              <strong>Step 1:</strong> Open the sign-in page and authorize access.
            </Typography>
            <Button
              variant="contained"
              color="primary"
              size="large"
              onClick={() => openExternalUrl(authUrl!)}
              fullWidth
            >
              Open Claude Sign-in Page
            </Button>
            <Typography variant="body2" sx={{ mt: 1 }}>
              <strong>Step 2:</strong> After authorizing, the page may show "can't be reached" &mdash;
              this is expected. Copy the <strong>full URL</strong> from your browser's address bar and paste it below.
            </Typography>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <TextField
                fullWidth
                size="small"
                placeholder="Paste the URL from your browser's address bar"
                value={authCode}
                onChange={(e) => setAuthCode(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleSubmitCode() }}
                disabled={codeSubmitting}
                autoFocus
              />
              <Button
                variant="contained"
                onClick={handleSubmitCode}
                disabled={!authCode.trim() || codeSubmitting}
              >
                {codeSubmitting ? <CircularProgress size={20} /> : 'Submit'}
              </Button>
            </Box>
          </Box>
        ) : (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4, gap: 2 }}>
            <CircularProgress />
            <Typography variant="body2" color="text.secondary">
              Completing authentication...
            </Typography>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  )
}

export default ClaudeSubscriptionConnect
