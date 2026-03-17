import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
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
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
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
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Start interactive login flow
  const handleStartLogin = useCallback(async () => {
    setLoginStarting(true)
    try {
      const result = await api.post<{ session_id: string }>('/api/v1/claude-subscriptions/start-login', {})
      if (result && result.session_id) {
        setLoginSessionId(result.session_id)
        setLoginDialogOpen(true)
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
                startIcon={<ErrorOutlineIcon />}
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
            {loginStarting ? 'Starting...' : 'Connect'}
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
              {loginStarting ? 'Starting...' : isExpired ? 'Re-authenticate' : 'Re-login'}
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

// Inner dialog component that manages the full login flow:
// 1. Poll until container is ready
// 2. Install/upgrade claude CLI (npm install)
// 3. Call get-login-url to start claude auth login in container, capture OAuth URL
// 4. Open OAuth URL in the user's native browser (window.open is intercepted by Wails on macOS)
// 5. Poll for credentials written to ~/.claude/.credentials.json
interface ClaudeLoginDialogInnerProps {
  sessionId: string
  open: boolean
  onClose: () => void
  pollIntervalRef: React.MutableRefObject<ReturnType<typeof setInterval> | null>
  onCredentialsCaptured: () => void
  orgId?: string
}

type LoginPhase =
  | 'waiting'       // container starting
  | 'installing'    // npm install running
  | 'opening'       // calling get-login-url, about to open browser
  | 'browser'       // browser opened, waiting for user to complete OAuth
  | 'error'         // flow failed

const ClaudeLoginDialogInner: FC<ClaudeLoginDialogInnerProps> = ({
  sessionId,
  open,
  onClose,
  pollIntervalRef,
  onCredentialsCaptured,
  orgId,
}) => {
  const api = useApi()
  const [phase, setPhase] = useState<LoginPhase>('waiting')
  const [loginUrl, setLoginUrl] = useState<string>('')
  const [isRunning, setIsRunning] = useState(false)
  const flowStartedRef = useRef(false)
  const pollingStartedRef = useRef(false)

  // Poll every 3s until the container reports running status (same logic as useSandboxState)
  useEffect(() => {
    const checkRunning = async () => {
      try {
        const apiClient = api.getApiClient()
        const response = await apiClient.v1SessionsDetail(sessionId)
        if (response.data) {
          const status = response.data.config?.external_agent_status || ''
          const desiredState = response.data.config?.desired_state || ''
          const hasContainer = !!response.data.config?.container_name
          if (status === 'running' || (hasContainer && desiredState === 'running')) {
            setIsRunning(true)
          }
        }
      } catch {
        // Not ready yet
      }
    }
    const interval = setInterval(checkRunning, 3000)
    checkRunning()
    return () => clearInterval(interval)
  }, [sessionId])

  // Once running, execute the login flow
  useEffect(() => {
    if (!isRunning || flowStartedRef.current) return
    flowStartedRef.current = true

    const runLoginFlow = async () => {
      try {
        const apiClient = api.getApiClient()

        // Upgrade claude CLI to latest. Old image versions used localhost:<random-port>
        // for OAuth which Anthropic no longer accepts; newer versions use
        // platform.claude.com/oauth/code/callback. Upgrading here self-heals deployed images.
        setPhase('installing')
        await apiClient.v1ExternalAgentsExecCreate(sessionId, {
          command: ['npm', 'install', '-g', '@anthropic-ai/claude-code@latest'],
          background: false,
          timeout: 300,
          env: {},
        })

        // Start claude auth login in the container via a fake browser that captures
        // the OAuth URL, then return it. This replaces the old approach of running
        // claude auth login with WAYLAND_DISPLAY and showing an embedded desktop stream.
        setPhase('opening')
        const urlResult = await apiClient.v1ClaudeSubscriptionsGetLoginUrlList({ session_id: sessionId })
        const oauthUrl = urlResult.data?.login_url
        if (!oauthUrl) {
          throw new Error('No login URL returned from container')
        }

        // Open in native browser. On macOS in the Wails app, window.open is intercepted
        // and routed to the system browser (Safari/Chrome). On web, opens a new tab.
        window.open(oauthUrl, '_blank')
        setLoginUrl(oauthUrl)
        setPhase('browser')
      } catch (err: any) {
        console.error('Claude login flow failed:', err)
        setPhase('error')
      }
    }

    runLoginFlow()
  }, [isRunning, sessionId])

  // Poll for credentials once the browser is open
  useEffect(() => {
    if (phase !== 'browser' || pollingStartedRef.current) return
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
            credentials: { claudeAiOauth: creds },
            ...(orgId ? { owner_type: 'org', owner_id: orgId } : {}),
          })

          if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current)
            pollIntervalRef.current = null
          }
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
  }, [phase, sessionId, orgId])

  const phaseLabel = (): string => {
    switch (phase) {
      case 'waiting':    return 'Starting session...'
      case 'installing': return 'Installing Claude CLI...'
      case 'opening':    return 'Opening browser...'
      case 'browser':    return 'Waiting for sign-in to complete...'
      case 'error':      return 'Failed to open browser'
    }
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Typography variant="h6">Sign in to Claude</Typography>
        <Typography variant="body2" color="text.secondary">
          Complete the login in your browser. Your credentials will automatically be reused in desktop sessions configured to use Claude Code.
        </Typography>
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, py: 1 }}>
          {phase === 'browser' ? (
            <>
              <Alert severity="info" icon={<OpenInNewIcon />}>
                A browser window has opened. Sign in to your Claude account and complete the authentication.
              </Alert>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <CircularProgress size={16} />
                <Typography variant="body2" color="text.secondary">
                  Waiting for sign-in to complete...
                </Typography>
              </Box>
              {loginUrl && (
                <Typography variant="caption" color="text.secondary">
                  If the browser did not open,{' '}
                  <a href={loginUrl} target="_blank" rel="noopener noreferrer">
                    click here to open the sign-in page
                  </a>
                </Typography>
              )}
            </>
          ) : phase === 'error' ? (
            <Alert severity="error">
              Failed to start the login flow. Please try again.
            </Alert>
          ) : (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <CircularProgress size={16} />
              <Typography variant="body2" color="text.secondary">
                {phaseLabel()}
              </Typography>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  )
}

export default ClaudeSubscriptionConnect
