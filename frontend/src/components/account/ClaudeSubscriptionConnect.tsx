import React, { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogActions from '@mui/material/DialogActions'
import CircularProgress from '@mui/material/CircularProgress'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import IconButton from '@mui/material/IconButton'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { getTokenExpiryStatus } from './claudeSubscriptionUtils'

interface ClaudeSubscriptionData {
  id: string
  created: string
  name: string
  credential_type: string
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
  variant?: 'button' | 'inline'
  onConnected?: () => void
  orgId?: string
}

const SETUP_TOKEN_COMMAND = 'claude setup-token'

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

  // Setup token dialog state
  const [tokenDialogOpen, setTokenDialogOpen] = useState(false)
  const [tokenValue, setTokenValue] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const handleOpenTokenDialog = () => {
    setTokenValue('')
    setSubmitError(null)
    setTokenDialogOpen(true)
  }

  const handleSubmitToken = async () => {
    const token = tokenValue.trim()
    if (!token) {
      setSubmitError('Please paste your setup token')
      return
    }

    setSubmitting(true)
    setSubmitError(null)
    try {
      await api.post('/api/v1/claude-subscriptions', {
        name: 'My Claude Subscription',
        setup_token: token,
        ...(orgId ? { owner_type: 'org', owner_id: orgId } : {}),
      })
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      snackbar.success('Claude subscription connected')
      setTokenDialogOpen(false)
      onConnected?.()
    } catch (err: any) {
      setSubmitError(err?.message || 'Failed to save token')
    } finally {
      setSubmitting(false)
    }
  }

  const handleCopyCommand = () => {
    navigator.clipboard.writeText(SETUP_TOKEN_COMMAND)
    snackbar.success('Command copied to clipboard')
  }

  const firstSub = subscriptions?.[0]
  const isSetupToken = firstSub?.credential_type === 'setup_token'
  const expiry = firstSub && !isSetupToken ? getTokenExpiryStatus(firstSub.access_token_expires_at) : null
  const isExpired = expiry?.isExpired ?? false

  // Token dialog (shared between both variants)
  const tokenDialog = (
    <Dialog open={tokenDialogOpen} onClose={() => setTokenDialogOpen(false)} maxWidth="sm" fullWidth>
      <DialogTitle>Connect Claude Subscription</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Generate a setup token on your local machine, then paste it below.
        </Typography>

        <Alert severity="info" sx={{ mb: 2 }}>
          <Typography variant="body2" gutterBottom>
            <strong>Step 1:</strong> Run this command in your terminal:
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', bgcolor: 'action.hover', borderRadius: 1, px: 1.5, py: 0.5, mt: 0.5, fontFamily: 'monospace', fontSize: '0.875rem' }}>
            <code style={{ flex: 1 }}>{SETUP_TOKEN_COMMAND}</code>
            <IconButton size="small" onClick={handleCopyCommand} title="Copy command">
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </Box>
          <Typography variant="body2" sx={{ mt: 1.5 }}>
            <strong>Step 2:</strong> Complete the authentication in your browser when prompted.
          </Typography>
          <Typography variant="body2" sx={{ mt: 1 }}>
            <strong>Step 3:</strong> Copy the token that appears and paste it below.
          </Typography>
        </Alert>

        <TextField
          autoFocus
          fullWidth
          type="password"
          label="Your Token"
          placeholder="Paste your token here..."
          value={tokenValue}
          onChange={(e) => setTokenValue(e.target.value)}
          variant="outlined"
          InputProps={{
            sx: { fontFamily: 'monospace', letterSpacing: '0.05em' },
          }}
          sx={{ mb: 1 }}
        />

        {submitError && (
          <Alert severity="error" sx={{ mb: 1 }}>
            {submitError}
          </Alert>
        )}

        <Alert severity="warning" sx={{ mt: 1 }}>
          <Typography variant="caption">
            To revoke this token later, visit{' '}
            <a href="https://claude.ai/settings/claude-code" target="_blank" rel="noopener noreferrer">
              claude.ai/settings/claude-code
            </a>
          </Typography>
        </Alert>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setTokenDialogOpen(false)}>Cancel</Button>
        <Button
          onClick={handleSubmitToken}
          variant="contained"
          disabled={submitting || !tokenValue.trim()}
        >
          {submitting ? <><CircularProgress size={14} sx={{ mr: 0.5 }} /> Connecting...</> : 'Connect'}
        </Button>
      </DialogActions>
    </Dialog>
  )

  // Disconnect dialog (shared)
  const disconnectDialog = (
    <Dialog open={disconnectDialogOpen} onClose={() => setDisconnectDialogOpen(false)}>
      <DialogTitle>Disconnect Claude Subscription</DialogTitle>
      <DialogContent>
        <DialogContentText>
          Are you sure you want to disconnect your Claude subscription?
          {isSetupToken && ' You may also want to revoke the token at claude.ai/settings/claude-code.'}
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
  )

  if (variant === 'button') {
    return (
      <>
        {hasSubscription ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.5 }}>
            {isExpired && !isSetupToken ? (
              <Button
                size="small"
                variant="contained"
                color="warning"
                onClick={handleOpenTokenDialog}
                startIcon={<ErrorOutlineIcon />}
              >
                Re-authenticate
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
            {expiry && !isSetupToken && (
              <Typography variant="caption" color={`${expiry.color}.main`} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {expiry.isExpiringSoon && !isExpired && <WarningAmberIcon sx={{ fontSize: 12 }} />}
                {isExpired && <ErrorOutlineIcon sx={{ fontSize: 12 }} />}
                {expiry.label}
              </Typography>
            )}
            {isSetupToken && (
              <Typography variant="caption" color="success.main" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <CheckCircleIcon sx={{ fontSize: 12 }} />
                Setup token
              </Typography>
            )}
          </Box>
        ) : (
          <Button
            size="small"
            variant="text"
            color="secondary"
            onClick={handleOpenTokenDialog}
          >
            Connect
          </Button>
        )}
        {disconnectDialog}
        {tokenDialog}
      </>
    )
  }

  // inline variant
  return (
    <>
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
        {hasSubscription ? (
          <>
            <CheckCircleIcon color="success" fontSize="small" />
            <Typography variant="body2" color="success.main">
              Claude subscription connected {isSetupToken ? '(setup token)' : ''}
            </Typography>
            {expiry && !isSetupToken && (
              <Typography variant="caption" color={`${expiry.color}.main`} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                ({expiry.label})
              </Typography>
            )}
            <Button
              size="small"
              variant="text"
              color="primary"
              onClick={handleOpenTokenDialog}
            >
              Update Token
            </Button>
          </>
        ) : (
          <Button
            variant="contained"
            color="secondary"
            onClick={handleOpenTokenDialog}
          >
            Connect with Setup Token
          </Button>
        )}
      </Box>
      {tokenDialog}
    </>
  )
}

export default ClaudeSubscriptionConnect
