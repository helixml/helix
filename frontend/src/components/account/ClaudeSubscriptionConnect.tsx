import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import CircularProgress from '@mui/material/CircularProgress'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import ExternalAgentDesktopViewer, { useSandboxState } from '../external-agent/ExternalAgentDesktopViewer'

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

  // Interactive login state
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [loginSessionId, setLoginSessionId] = useState<string>('')
  const [loginStarting, setLoginStarting] = useState(false)
  const [loginCommandSent, setLoginCommandSent] = useState(false)
  const [loginPolling, setLoginPolling] = useState(false)
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

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

  if (variant === 'button') {
    return (
      <>
        <Button
          size="small"
          variant={hasSubscription ? 'outlined' : 'text'}
          color={hasSubscription ? 'success' : 'secondary'}
          onClick={handleStartLogin}
          startIcon={hasSubscription ? <CheckCircleIcon /> : undefined}
          disabled={loginStarting}
        >
          {loginStarting ? 'Starting...' : hasSubscription ? 'Connected' : 'Connect'}
        </Button>

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
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
        {hasSubscription ? (
          <>
            <CheckCircleIcon color="success" fontSize="small" />
            <Typography variant="body2" color="success.main">Claude subscription connected</Typography>
            <Button size="small" onClick={handleStartLogin} disabled={loginStarting}>
              {loginStarting ? 'Starting...' : 'Re-login'}
            </Button>
          </>
        ) : (
          <Button
            variant="contained"
            color="secondary"
            onClick={handleStartLogin}
            disabled={loginStarting}
          >
            {loginStarting ? 'Starting...' : 'Login with Browser'}
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
            WAYLAND_DISPLAY: 'wayland-0',
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
  // Use a ref guard instead of state in deps to prevent the effect cleanup
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
            name: 'My Claude Subscription',
            credentials: {
              claudeAiOauth: creds,
            },
            ...(orgId ? { owner_type: 'org', owner_id: orgId } : {}),
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
  }, [loginCommandSent, sessionId, orgId])

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: { height: '80vh', maxHeight: '80vh' },
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

export default ClaudeSubscriptionConnect
