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
import Switch from '@mui/material/Switch'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Tooltip from '@mui/material/Tooltip'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { getTokenExpiryStatus } from './claudeSubscriptionUtils'
import { matchesAllTokens } from '../../utils/searchUtils'

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
  // Orgs allowed to run orchestrated agents on this subscription for its owner.
  // Empty means it is only ever used for sessions the owner owns.
  delegated_org_ids?: string[]
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

// Above this many orgs the list gets a filter box. Below it, scanning is faster
// than typing.
const DELEGATION_FILTER_THRESHOLD = 8

interface DelegationPickerProps {
  organizations: { id?: string; name?: string; display_name?: string }[]
  delegatedOrgIDs: string[]
  disabled: boolean
  onToggle: (orgID: string, enabled: boolean) => void
}

// DelegationPicker lists the orgs whose agents may run on this subscription.
// It is scroll-capped because membership is unbounded — someone in 27 orgs was
// getting 27 stacked switches, which pushed the card's own buttons a screen and
// a half down. Granted orgs sort to the top so the answer to "who can spend my
// quota" is visible without scrolling or hunting.
const DelegationPicker: FC<DelegationPickerProps> = ({
  organizations,
  delegatedOrgIDs,
  disabled,
  onToggle,
}) => {
  const [filter, setFilter] = useState('')

  const withIDs = organizations.filter((org) => !!org.id)
  const labelFor = (org: { id?: string; name?: string; display_name?: string }) =>
    org.display_name || org.name || (org.id as string)

  const sorted = [...withIDs].sort((a, b) => {
    const aOn = delegatedOrgIDs.includes(a.id as string)
    const bOn = delegatedOrgIDs.includes(b.id as string)
    if (aOn !== bOn) return aOn ? -1 : 1
    return labelFor(a).localeCompare(labelFor(b))
  })
  const visible = sorted.filter((org) => matchesAllTokens(filter, labelFor(org), org.id))
  const grantedCount = withIDs.filter((org) => delegatedOrgIDs.includes(org.id as string)).length

  return (
    <Box sx={{ mt: 1.5 }}>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
        Let these organizations&apos; agents use this subscription when they run work on your
        behalf — for example a bot you own that is launched by a shared service account. Without
        this, your subscription is only used for sessions you own.
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
        {grantedCount === 0
          ? `Not shared with any of your ${withIDs.length} organizations.`
          : `Shared with ${grantedCount} of ${withIDs.length} organizations.`}
      </Typography>
      {withIDs.length > DELEGATION_FILTER_THRESHOLD && (
        <TextField
          size="small"
          fullWidth
          placeholder="Filter organizations"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          sx={{ mt: 1, maxWidth: 320 }}
        />
      )}
      <Box
        sx={{
          mt: 0.5,
          // Caps the card at a readable height however many orgs you are in.
          maxHeight: 200,
          overflowY: 'auto',
          pr: 1,
        }}
      >
        <FormGroup>
          {visible.map((org) => {
            const orgID = org.id as string
            return (
              <FormControlLabel
                key={orgID}
                control={
                  <Switch
                    size="small"
                    checked={delegatedOrgIDs.includes(orgID)}
                    disabled={disabled}
                    onChange={(e) => onToggle(orgID, e.target.checked)}
                  />
                }
                label={<Typography variant="caption">{labelFor(org)}</Typography>}
              />
            )
          })}
        </FormGroup>
        {visible.length === 0 && (
          <Typography variant="caption" color="text.secondary">
            No organizations match &quot;{filter}&quot;.
          </Typography>
        )}
      </Box>
    </Box>
  )
}

const SETUP_TOKEN_COMMAND = 'claude setup-token'

function validateSetupToken(token: string): string | null {
  const trimmed = token.trim()
  if (!trimmed) return null

  if (trimmed.startsWith('sk-ant-api')) {
    return 'This looks like an Anthropic API key, not a Claude Code setup token. Run `claude setup-token` in your terminal to generate the correct token.'
  }

  if (!trimmed.startsWith('sk-ant-oat')) {
    return "This doesn't look like a valid Claude Code setup token. Run `claude setup-token` to generate one."
  }

  if (trimmed.length < 50) {
    return 'This token appears to be incomplete. Make sure you copied the full token.'
  }

  return null
}

const ClaudeSubscriptionConnect: FC<ClaudeSubscriptionConnectProps> = ({
  variant = 'button',
  onConnected,
  orgId,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const lightTheme = useLightTheme()
  const account = useAccount()

  const organizations = account.organizationTools.organizations || []

  const { data: subscriptions, isLoading } = useClaudeSubscriptions()
  const hasSubscription = subscriptions && subscriptions.length > 0

  // Only an org owner may connect (or delete) an org-level subscription, so the
  // owner picker must not offer orgs where this person is a plain member.
  const currentUserID = account.user?.id
  const ownableOrgs = organizations.filter(
    (org) =>
      !!org.id &&
      (account.admin ||
        (org.memberships || []).some((m) => m.user_id === currentUserID && m.role === 'owner')),
  )
  const orgLabel = (orgID: string) => {
    const org = organizations.find((o) => o.id === orgID)
    return org?.display_name || org?.name || orgID
  }
  const personalSub = subscriptions?.find((s) => s.owner_type === 'user')
  const subForOwner = (type: 'user' | 'org', orgID: string) =>
    type === 'user' ? personalSub : subscriptions?.find((s) => s.owner_type === 'org' && s.owner_id === orgID)

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

  // Delegation: let an org's orchestrated agents (e.g. a bot dispatched under a
  // service account) authenticate as this subscription's owner. Owner-only, and
  // opt-in per org — without it nobody else's automation can spend your quota.
  const delegationMutation = useMutation({
    mutationFn: async ({ id, orgIDs }: { id: string; orgIDs: string[] }) => {
      return api.put(`/api/v1/claude-subscriptions/${id}/delegation`, {
        delegated_org_ids: orgIDs,
      }, {}, { snackbar: true })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
    },
  })

  const toggleDelegation = (sub: ClaudeSubscriptionData, orgID: string, enabled: boolean) => {
    const current = sub.delegated_org_ids || []
    const next = enabled
      ? Array.from(new Set([...current, orgID]))
      : current.filter((id) => id !== orgID)
    delegationMutation.mutate({ id: sub.id, orgIDs: next })
  }

  // Setup token dialog state
  const [tokenDialogOpen, setTokenDialogOpen] = useState(false)
  const [tokenValue, setTokenValue] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Org selector state (used by account variant)
  const [ownerType, setOwnerType] = useState<'user' | 'org'>('user')
  const [selectedOrgId, setSelectedOrgId] = useState('')
  // When re-authenticating an existing subscription the owner is fixed — you are
  // replacing that credential, not choosing where a new one lands.
  const [ownerLocked, setOwnerLocked] = useState(false)

  const handleOpenTokenDialog = () => {
    setTokenValue('')
    setSubmitError(null)
    setOwnerType('user')
    setSelectedOrgId('')
    setOwnerLocked(false)
    setTokenDialogOpen(true)
  }

  // Re-authenticate a specific subscription: pin the dialog to its owner.
  const handleOpenTokenDialogFor = (sub: ClaudeSubscriptionData) => {
    setTokenValue('')
    setSubmitError(null)
    setOwnerType(sub.owner_type === 'org' ? 'org' : 'user')
    setSelectedOrgId(sub.owner_type === 'org' ? sub.owner_id : '')
    setOwnerLocked(true)
    setTokenDialogOpen(true)
  }

  const handleSubmitToken = async () => {
    const token = tokenValue.trim()
    if (!token) {
      setSubmitError('Please paste your setup token')
      return
    }

    if (!orgId && ownerType === 'org' && !selectedOrgId) {
      setSubmitError('Please choose which organization owns this subscription')
      return
    }

    setSubmitting(true)
    setSubmitError(null)
    try {
      // Use orgId prop if provided (button/inline variants), otherwise use internal state (account variant)
      const effectiveOrgId = orgId || (ownerType === 'org' ? selectedOrgId : undefined)
      await api.post('/api/v1/claude-subscriptions', {
        name: effectiveOrgId ? `${orgLabel(effectiveOrgId)} Claude Subscription` : 'My Claude Subscription',
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

  const tokenValidationError = validateSetupToken(tokenValue)

  const firstSub = subscriptions?.[0]
  const isSetupToken = firstSub?.credential_type === 'setup_token'
  const expiry = firstSub && !isSetupToken ? getTokenExpiryStatus(firstSub.access_token_expires_at) : null
  const isExpired = expiry?.isExpired ?? false

  // Owner picker — only meaningful in the account variant, where you manage every
  // subscription you can see. The other variants are scoped by the `orgId` prop.
  // An org-owned subscription is the shared fallback: any member's session that
  // has no personal subscription uses it, so say so rather than leaving people to
  // infer it from the word "organization".
  const replacedSub = ownerLocked ? undefined : subForOwner(ownerType, selectedOrgId)
  const ownerPicker = variant === 'account' && ownableOrgs.length > 0 && (
    <Box sx={{ mb: 2 }}>
      <FormControl size="small" fullWidth disabled={ownerLocked}>
        <InputLabel>Subscription owner</InputLabel>
        <Select
          value={ownerType === 'org' ? selectedOrgId : 'personal'}
          label="Subscription owner"
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
          <MenuItem value="personal">Personal — only my own sessions</MenuItem>
          {ownableOrgs.map((org) => (
            <MenuItem key={org.id} value={org.id as string}>
              {org.display_name || org.name} — shared with the whole organization
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
        {ownerLocked
          ? 'Replacing the credentials on this subscription. The owner cannot be changed — disconnect it and connect a new one to move it.'
          : ownerType === 'org'
            ? `Anyone in ${orgLabel(selectedOrgId)} whose session has no personal Claude subscription will run on this one, and it will be billed to the Claude account you authenticate below.`
            : 'Used only for sessions you own, unless you delegate it to an organization afterwards.'}
      </Typography>
      {replacedSub && (
        <Alert severity="warning" sx={{ mt: 1 }} icon={false}>
          <Typography variant="caption">
            {ownerType === 'org' ? orgLabel(selectedOrgId) : 'Your account'} already has a
            connected subscription. Connecting a new token will replace it.
          </Typography>
        </Alert>
      )}
    </Box>
  )

  // Token dialog (shared across all variants)
  const tokenDialog = (
    <Dialog open={tokenDialogOpen} onClose={() => setTokenDialogOpen(false)} maxWidth="sm" fullWidth>
      <DialogTitle>{ownerLocked ? 'Update Claude Subscription' : 'Connect Claude Subscription'}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Generate a setup token on your local machine, then paste it below.
        </Typography>

        {ownerPicker}

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
          label="Claude Code Setup Token"
          placeholder="Paste your Claude Code setup token here..."
          value={tokenValue}
          onChange={(e) => setTokenValue(e.target.value)}
          variant="outlined"
          InputProps={{
            sx: { fontFamily: 'monospace', letterSpacing: '0.05em' },
          }}
          sx={{ mb: 1 }}
        />

        {tokenValidationError && (
          <Alert severity="error" sx={{ mb: 1 }}>
            {tokenValidationError}
          </Alert>
        )}

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
          disabled={submitting || !tokenValue.trim() || !!tokenValidationError}
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
        <Grid container spacing={2} sx={{ mt: 2, backgroundColor: lightTheme.panelColor, p: 2, borderRadius: 2 }}>
          <Grid item xs={12}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Box>
                <Typography variant="h6">Claude Code Subscription</Typography>
                <Typography variant="body2" color="text.secondary">
                  Connect your Claude subscription to use Claude Code as the coding agent in Helix desktop sessions.
                  {ownableOrgs.length > 0 && ' You can also connect one for an organization you own, as a shared fallback for members who have not connected their own.'}
                </Typography>
              </Box>
              {/* With no orgs you own there is exactly one subscription you can
                  hold, and the card's "Update Token" is how you change it. */}
              {(!hasSubscription || ownableOrgs.length > 0) && (
                <Button
                  variant="contained"
                  color="secondary"
                  onClick={handleOpenTokenDialog}
                  sx={{ flexShrink: 0 }}
                >
                  {hasSubscription ? 'Add subscription' : 'Connect with Setup Token'}
                </Button>
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
                      // Top-aligned, not centred: the delegation list makes this row
                      // tall, and centring stranded Update Token / delete halfway down
                      // the card, far from the subscription they act on.
                      alignItems: 'flex-start',
                      gap: 2,
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
                        {sub.subscription_type && (
                          <Chip label={sub.subscription_type} size="small" variant="outlined" />
                        )}
                        {sub.owner_type === 'org' ? (
                          <Chip
                            label={`Shared: ${orgLabel(sub.owner_id)}`}
                            size="small"
                            variant="outlined"
                            color="info"
                          />
                        ) : (
                          <Chip label="Personal" size="small" variant="outlined" />
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
                      {sub.owner_type === 'org' && (
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                          Shared fallback for {orgLabel(sub.owner_id)}: used by any member&apos;s
                          session that has no personal subscription of its own.
                        </Typography>
                      )}
                      {variant === 'account' && sub.owner_type === 'user' && organizations.length > 0 && (
                        <DelegationPicker
                          organizations={organizations}
                          delegatedOrgIDs={sub.delegated_org_ids || []}
                          disabled={delegationMutation.isPending}
                          onToggle={(orgID, enabled) => toggleDelegation(sub, orgID, enabled)}
                        />
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                      <Button
                        variant={subIsExpired ? 'contained' : 'outlined'}
                        color={subIsExpired ? 'warning' : 'secondary'}
                        size="small"
                        onClick={() => handleOpenTokenDialogFor(sub)}
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
                  No Claude subscription connected. Click &quot;Connect with Setup Token&quot; to get started
                  {ownableOrgs.length > 0 && ' — you can connect it for yourself, or for an organization you own'}.
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
    const connectButtonSx = { textTransform: 'none', minWidth: 0, px: 1 } as const
    return (
      <>
        {hasSubscription ? (
          isExpired && !isSetupToken ? (
            <Button
              size="small"
              variant="contained"
              color="warning"
              onClick={handleOpenTokenDialog}
              startIcon={<ErrorOutlineIcon />}
              sx={connectButtonSx}
            >
              Re-authenticate
            </Button>
          ) : (
            <Tooltip title={expiry && !isSetupToken ? expiry.label : ''} disableHoverListener={!expiry || isSetupToken}>
              <Button
                size="small"
                variant="text"
                color="error"
                onClick={() => {
                  if (subscriptions?.[0]?.id) {
                    setDeleteTarget(subscriptions[0].id)
                    setDisconnectDialogOpen(true)
                  }
                }}
                sx={connectButtonSx}
              >
                Disconnect
              </Button>
            </Tooltip>
          )
        ) : (
          <Button
            size="small"
            variant="text"
            color="secondary"
            onClick={handleOpenTokenDialog}
            sx={connectButtonSx}
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
