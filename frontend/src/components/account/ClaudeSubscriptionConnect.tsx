import React, { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Grid from '@mui/material/Grid'
import Chip from '@mui/material/Chip'
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
import DeleteIcon from '@mui/icons-material/Delete'
import IconButton from '@mui/material/IconButton'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'
import useAccount from '../../hooks/useAccount'
import { getTokenExpiryStatus } from './claudeSubscriptionUtils'

interface ClaudeSubscriptionData {
  id: string
  created: string
  name: string
  credential_type?: string
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
  variant?: 'button' | 'inline' | 'account'
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
  const themeConfig = useThemeConfig()
  const account = useAccount()

  const organizations = account.organizationTools.organizations || []

  const { data: subscriptions, isLoading } = useClaudeSubscriptions()
  const hasSubscription = subscriptions && subscriptions.length > 0

  // Disconnect / delete state
  const [disconnectDialogOpen, setDisconnectDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string>('')
  const disconnectMutation = useMutation({
    mutationFn: async (id: string) => {
      return api.delete(`/api/v1/claude-subscriptions/${id}`, {}, {
        snackbar: true,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      setDisconnectDialogOpen(false)
      setDeleteTarget('')
      snackbar.success('Claude subscription disconnected')
    },
  })

  // Setup token dialog state
  const [tokenDialogOpen, setTokenDialogOpen] = useState(false)
  const [tokenValue, setTokenValue] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Org selector state (used by account variant)
  const [ownerType, setOwnerType] = useState<'user' | 'org'>('user')
  const [selectedOrgId, setSelectedOrgId] = useState('')

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
      // Use orgId prop if provided (button/inline variants), otherwise use internal state (account variant)
      const effectiveOrgId = orgId || (ownerType === 'org' ? selectedOrgId : undefined)
      await api.post('/api/v1/claude-subscriptions', {
        name: 'My Claude Subscription',
        setup_token: token,
        ...(effectiveOrgId ? { owner_type: 'org', owner_id: effectiveOrgId } : {}),
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

  const handleDeleteClick = (id: string) => {
    setDeleteTarget(id)
    setDisconnectDialogOpen(true)
  }

  const handleConfirmDelete = () => {
    if (deleteTarget) {
      disconnectMutation.mutate(deleteTarget)
    }
  }

  const firstSub = subscriptions?.[0]
  const isSetupToken = firstSub?.credential_type === 'setup_token'
  const expiry = firstSub && !isSetupToken ? getTokenExpiryStatus(firstSub.access_token_expires_at) : null
  const isExpired = expiry?.isExpired ?? false

  // Token dialog (shared across all variants)
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

  // Disconnect dialog (shared across all variants)
  const disconnectDialog = (
    <Dialog open={disconnectDialogOpen} onClose={() => setDisconnectDialogOpen(false)}>
      <DialogTitle>Disconnect Claude Subscription</DialogTitle>
      <DialogContent>
        <DialogContentText>
          Are you sure you want to disconnect this Claude subscription?
          {' '}You may also want to revoke the token at claude.ai/settings/claude-code.
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setDisconnectDialogOpen(false)}>Cancel</Button>
        <Button
          onClick={handleConfirmDelete}
          color="error"
          variant="contained"
          disabled={disconnectMutation.isPending}
        >
          {disconnectMutation.isPending ? 'Disconnecting...' : 'Disconnect'}
        </Button>
      </DialogActions>
    </Dialog>
  )

  // --- Account variant: full subscription list with org selector ---
  if (variant === 'account') {
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
                    onClick={handleOpenTokenDialog}
                  >
                    Connect with Setup Token
                  </Button>
                </Box>
              )}
            </Box>

            {isLoading ? (
              <Typography variant="body2" color="text.secondary">Loading...</Typography>
            ) : hasSubscription ? (
              subscriptions.map((sub) => {
                const subIsSetupToken = sub.credential_type === 'setup_token'
                const subExpiry = subIsSetupToken ? null : getTokenExpiryStatus(sub.access_token_expires_at)
                const subIsExpired = subExpiry?.isExpired ?? false
                return (
                  <Box
                    key={sub.id}
                    sx={{
                      p: 2,
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: subIsExpired ? 'error.main' : subExpiry?.isExpiringSoon ? 'warning.main' : 'divider',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      mb: 1,
                    }}
                  >
                    <Box>
                      <Typography variant="subtitle1">{sub.name || 'Claude Subscription'}</Typography>
                      <Box sx={{ display: 'flex', gap: 1, mt: 0.5, alignItems: 'center', flexWrap: 'wrap' }}>
                        {subIsExpired && !subIsSetupToken ? (
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
                        {subIsSetupToken && (
                          <Chip label="Setup Token" size="small" variant="outlined" />
                        )}
                        {sub.subscription_type && (
                          <Chip label={sub.subscription_type} size="small" variant="outlined" />
                        )}
                        {sub.owner_type === 'org' && (
                          <Chip label="Organization" size="small" variant="outlined" />
                        )}
                        {subExpiry && (
                          <Typography
                            variant="caption"
                            color={`${subExpiry.color}.main`}
                            sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                          >
                            {subExpiry.isExpiringSoon && !subIsExpired && <WarningAmberIcon sx={{ fontSize: 14 }} />}
                            {subExpiry.label}
                          </Typography>
                        )}
                      </Box>
                      {subIsExpired && !subIsSetupToken && (
                        <Alert severity="warning" sx={{ mt: 1, py: 0 }} icon={false}>
                          <Typography variant="caption">
                            Token has expired. Update your token to refresh credentials for new sessions.
                          </Typography>
                        </Alert>
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                      <Button
                        variant={subIsExpired ? 'contained' : 'outlined'}
                        color={subIsExpired ? 'warning' : 'secondary'}
                        size="small"
                        onClick={handleOpenTokenDialog}
                      >
                        {subIsExpired ? 'Re-authenticate' : 'Update Token'}
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
                  No Claude subscription connected. Click &quot;Connect with Setup Token&quot; to get started.
                </Typography>
              </Box>
            )}
          </Grid>
        </Grid>

        {tokenDialog}
        {disconnectDialog}
      </>
    )
  }

  // --- Button variant ---
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
                onClick={() => {
                  if (subscriptions?.[0]?.id) {
                    setDeleteTarget(subscriptions[0].id)
                    setDisconnectDialogOpen(true)
                  }
                }}
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

  // --- Inline variant ---
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
