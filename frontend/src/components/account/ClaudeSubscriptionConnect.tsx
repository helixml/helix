import React, { FC, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Chip from '@mui/material/Chip'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogActions from '@mui/material/DialogActions'
import CircularProgress from '@mui/material/CircularProgress'

import IconButton from '@mui/material/IconButton'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import Switch from '@mui/material/Switch'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Tooltip from '@mui/material/Tooltip'
import { CircleAlert, CircleCheck, Copy, Trash2 } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useLightTheme from '../../hooks/useLightTheme'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'
import { TypesOwnerType } from '../../api/api'
import ClaudeConnectMethodPicker, { ClaudeConnectMethod } from './ClaudeConnectMethodPicker'
import { SELECT_MENU_PROPS } from '../../contexts/theme'
import useAccount from '../../hooks/useAccount'
import { formatClaudeAccountDetail, formatClaudeOrganizationRef } from './claudeSubscriptionUtils'
import SubscriptionIdentity from './SubscriptionIdentity'
import { matchesAllTokens } from '../../utils/searchUtils'
import { APP_MONO_FONT_FAMILY } from '../../styles/typography'
import { codeAgentHarnessesQueryKey } from '../../services/codeAgentHarnessesService'

interface ClaudeSubscriptionData {
  id: string
  created: string
  name: string
  credential_type?: string
  subscription_type: string
  rate_limit_tier: string
  account_email?: string
  account_display_name?: string
  claude_organization_id?: string
  status: string
  access_token_expires_at: string
  // When the login itself dies and the user must sign in again. Zero/absent for
  // setup tokens, which carry no refresh token.
  refresh_token_expires_at?: string
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
  enableForOrgId?: string
  // Set when the account variant renders inside a harness row that already
  // names and frames the harness — drops the duplicate panel and heading.
  embedded?: boolean
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
          mt: 1,
          // Caps the card at a readable height however many orgs you are in.
          maxHeight: 200,
          overflowY: 'auto',
          // Both sides need padding: the scroll container clips, and MUI's
          // switch draws its ripple outside its own box.
          px: 0.5,
        }}
      >
        <FormGroup sx={{ rowGap: 0.5 }}>
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
                // Default is marginLeft:-11px, which the scroll box cuts off.
                sx={{ ml: 0, mr: 0, gap: 1 }}
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
const CREDENTIALS_FILE_COMMAND = 'cat ~/.claude/.credentials.json'

// Shape of ~/.claude/.credentials.json, written by `claude login` on the user's
// own machine. Accepting it is what lets Helix reuse a login the user already
// did, the way local tools do — no OAuth flow of our own.
interface ClaudeOAuthCredentialsInput {
  accessToken: string
  refreshToken: string
  expiresAt?: number
  scopes?: string[]
  subscriptionType?: string
  rateLimitTier?: string
}

// Accepts the whole credentials file or just the claudeAiOauth object inside it,
// because people copy either one.
export function parseClaudeCredentials(value: string): ClaudeOAuthCredentialsInput {
  let parsed: any
  try {
    parsed = JSON.parse(value)
  } catch {
    throw new Error('That is not valid JSON. Paste the contents of ~/.claude/.credentials.json.')
  }
  const creds = parsed?.claudeAiOauth ?? parsed
  if (!creds?.accessToken || !creds?.refreshToken) {
    throw new Error(
      'This is not a complete Claude credentials file — it needs accessToken and refreshToken.',
    )
  }
  return creds as ClaudeOAuthCredentialsInput
}

const StepIndex: FC<{ n: number }> = ({ n }) => (
  <Box
    aria-hidden
    sx={{
      width: 22,
      height: 22,
      mt: '1px',
      borderRadius: '50%',
      bgcolor: 'action.selected',
      color: 'text.primary',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontSize: '0.75rem',
      fontWeight: 600,
      flexShrink: 0,
    }}
  >
    {n}
  </Box>
)

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
  enableForOrgId,
  embedded = false,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const lightTheme = useLightTheme()
  const muiTheme = useTheme()
  const dialogFullScreen = useMediaQuery(muiTheme.breakpoints.down('sm'))
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
  // Multi-word search (AND across tokens), matching the rest of the app's
  // filter boxes rather than a naive substring test.
  const [ownerFilter, setOwnerFilter] = useState('')
  const filteredOwnableOrgs = ownerFilter.trim()
    ? ownableOrgs.filter((org) => matchesAllTokens(org.display_name || org.name || '', ownerFilter))
    : ownableOrgs

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
    mutationFn: async (id: string) => (await api.getApiClient().v1ClaudeSubscriptionsDelete(id)).data,
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
  // Credentials-file import is the default: it is the only path that yields the
  // account email and plan, because those tokens carry the user:profile scope
  // that setup tokens lack.
  const [connectMethod, setConnectMethod] = useState<ClaudeConnectMethod>('oauth')
  const [credentialsValue, setCredentialsValue] = useState('')
  // PKCE material for an in-flight sign-in. Held in component state because the
  // browser is the OAuth client: it started the login and it finishes it.
  const [oauthChallenge, setOauthChallenge] = useState<{ url: string; verifier: string; state: string } | null>(null)
  const [oauthCode, setOauthCode] = useState('')
  const [startingOauth, setStartingOauth] = useState(false)
  const [tokenValue, setTokenValue] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const resetDialogState = () => {
    setTokenValue('')
    setCredentialsValue('')
    setConnectMethod('oauth')
    setOauthChallenge(null)
    setOauthCode('')
    setSubmitError(null)
  }

  // Org selector state (used by account variant)
  const [ownerType, setOwnerType] = useState<'user' | 'org'>('user')
  const [selectedOrgId, setSelectedOrgId] = useState('')
  // When re-authenticating an existing subscription the owner is fixed — you are
  // replacing that credential, not choosing where a new one lands.
  const [ownerLocked, setOwnerLocked] = useState(false)

  const handleOpenTokenDialog = () => {
    resetDialogState()
    setOwnerType('user')
    setSelectedOrgId('')
    setOwnerLocked(false)
    setTokenDialogOpen(true)
  }

  // Re-authenticate a specific subscription: pin the dialog to its owner.
  const handleOpenTokenDialogFor = (sub: ClaudeSubscriptionData) => {
    resetDialogState()
    setOwnerType(sub.owner_type === 'org' ? 'org' : 'user')
    setSelectedOrgId(sub.owner_type === 'org' ? sub.owner_id : '')
    setOwnerLocked(true)
    setTokenDialogOpen(true)
  }

  const handleStartOauth = async () => {
    // If a previous attempt left a code in the field it belongs to the old
    // challenge; submitting it against the new verifier would fail obscurely.
    if (oauthChallenge) {
      // Already have a live challenge — reopen it rather than minting another,
      // so a code the user has already been shown stays valid.
      window.open(oauthChallenge.url, '_blank', 'noopener,noreferrer')
      return
    }
    setOauthCode('')
    setStartingOauth(true)
    setSubmitError(null)
    try {
      const { data } = await api.getApiClient().v1ClaudeSubscriptionsOauthStartCreate()
      setOauthChallenge({ url: data.authorize_url || '', verifier: data.code_verifier || '', state: data.state || '' })
      window.open(data.authorize_url, '_blank', 'noopener,noreferrer')
    } catch (e: any) {
      setSubmitError(e?.response?.data?.error || 'Could not start the Claude sign-in')
    } finally {
      setStartingOauth(false)
    }
  }

  const handleSubmitToken = async () => {
    if (!orgId && ownerType === 'org' && !selectedOrgId) {
      setSubmitError('Please choose which organization owns this subscription')
      return
    }
    const effectiveOrgIdForOauth = orgId || (ownerType === 'org' ? selectedOrgId : undefined)

    if (connectMethod === 'oauth') {
      if (!oauthChallenge) {
        setSubmitError('Start the sign-in first')
        return
      }
      if (!oauthCode.trim()) {
        setSubmitError('Paste the code Anthropic showed you after signing in')
        return
      }
      setSubmitting(true)
      setSubmitError(null)
      try {
        await api.getApiClient().v1ClaudeSubscriptionsOauthCompleteCreate({
          code: oauthCode.trim(),
          code_verifier: oauthChallenge.verifier,
          state: oauthChallenge.state,
          name: effectiveOrgIdForOauth
            ? `${orgLabel(effectiveOrgIdForOauth)} Claude Subscription`
            : 'My Claude Subscription',
          ...(enableForOrgId ? { organization_id: enableForOrgId } : {}),
          ...(effectiveOrgIdForOauth ? { owner_type: TypesOwnerType.OwnerTypeOrg, owner_id: effectiveOrgIdForOauth } : {}),
        })
        queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
        if (enableForOrgId) {
          queryClient.invalidateQueries({ queryKey: codeAgentHarnessesQueryKey(enableForOrgId) })
        }
        snackbar.success('Claude subscription connected')
        resetDialogState()
        setTokenDialogOpen(false)
        onConnected?.()
      } catch (e: any) {
        setSubmitError(e?.response?.data?.error || 'Could not complete the Claude sign-in')
      } finally {
        setSubmitting(false)
      }
      return
    }

    let credentialPayload: Record<string, unknown>
    if (connectMethod === 'credentials') {
      if (!credentialsValue.trim()) {
        setSubmitError('Please paste the contents of ~/.claude/.credentials.json')
        return
      }
      try {
        credentialPayload = { credentials: { claudeAiOauth: parseClaudeCredentials(credentialsValue) } }
      } catch (e) {
        setSubmitError(e instanceof Error ? e.message : 'Could not read those credentials')
        return
      }
    } else {
      const token = tokenValue.trim()
      if (!token) {
        setSubmitError('Please paste your setup token')
        return
      }
      credentialPayload = { setup_token: token }
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
      await api.getApiClient().v1ClaudeSubscriptionsCreate({
        name: effectiveOrgId ? `${orgLabel(effectiveOrgId)} Claude Subscription` : 'My Claude Subscription',
        ...credentialPayload,
        ...(enableForOrgId ? { organization_id: enableForOrgId } : {}),
        ...(effectiveOrgId ? { owner_type: TypesOwnerType.OwnerTypeOrg, owner_id: effectiveOrgId } : {}),
      })
      queryClient.invalidateQueries({ queryKey: ['claude-subscriptions'] })
      if (enableForOrgId) {
        queryClient.invalidateQueries({ queryKey: codeAgentHarnessesQueryKey(enableForOrgId) })
      }
      snackbar.success('Claude subscription connected')
      resetDialogState()
      setTokenDialogOpen(false)
      onConnected?.()
    } catch (err: any) {
      setSubmitError(err?.response?.data?.error || err?.message || 'Failed to save token')
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

  const tokenValidationError = connectMethod === 'setup_token' ? validateSetupToken(tokenValue) : ''

  const firstSub = subscriptions?.[0]
  const isSetupToken = firstSub?.credential_type === 'setup_token'
  // See the card above: status, not the clock, is what says a credential needs
  // the user's attention now that refresh happens in the background.
  const isExpired = !!firstSub && firstSub.status !== 'active'

  // Owner picker — only meaningful in the account variant, where you manage every
  // subscription you can see. The other variants are scoped by the `orgId` prop.
  // An org-owned subscription is the shared fallback: any member's session that
  // has no personal subscription uses it, so say so rather than leaving people to
  // infer it from the word "organization".
  const replacedSub = ownerLocked ? undefined : subForOwner(ownerType, selectedOrgId)
  const ownerPicker = variant === 'account' && ownableOrgs.length > 0 && (
    <Box>
      {/* Re-authenticating replaces the credential on an existing subscription,
          so the owner is fixed. A disabled dropdown just looks broken — state
          the owner and explain it below instead. */}
      {ownerLocked ? (
        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            Subscription owner
          </Typography>
          <Typography variant="body2">
            {ownerType === 'org' ? orgLabel(selectedOrgId) : 'Personal — only my own sessions'}
          </Typography>
        </Box>
      ) : (
      <FormControl size="small" fullWidth>
        <InputLabel>Subscription owner</InputLabel>
        <Select
          value={ownerType === 'org' ? selectedOrgId : 'personal'}
          label="Subscription owner"
          // autoFocus off so the menu does not steal focus from the search box
          // for the selected item; everything else is the shared behaviour.
          MenuProps={{ ...SELECT_MENU_PROPS, autoFocus: false }}
          onClose={() => setOwnerFilter('')}
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
          <Box
            sx={{
              position: 'sticky',
              top: 0,
              zIndex: 1,
              px: 1,
              pb: 1,
              bgcolor: 'background.paper',
            }}
          >
            <TextField
              size="small"
              fullWidth
              autoFocus
              placeholder="Search organizations"
              value={ownerFilter}
              onChange={(e) => setOwnerFilter(e.target.value)}
              // Select's type-ahead would eat the characters typed here, but
              // swallowing everything also killed Escape (MUI's Modal handles it
              // as a synthetic onKeyDown) and the arrows that move into the list.
              onKeyDown={(e) => {
                if (e.key === 'Escape' || e.key === 'Tab' || e.key.startsWith('Arrow')) return
                e.stopPropagation()
              }}
            />
          </Box>
          <MenuItem value="personal">Personal — only my own sessions</MenuItem>
          {ownableOrgs.map((org) => {
            const orgID = org.id as string
            const visible = filteredOwnableOrgs.some((candidate) => candidate.id === orgID)
            // The selected item must stay mounted even when filtered out, or
            // MUI cannot resolve the value to a label and renders the control
            // blank while you type.
            const isSelected = ownerType === 'org' && selectedOrgId === orgID
            if (!visible && !isSelected) return null
            return (
              <MenuItem
                key={orgID}
                value={orgID}
                sx={!visible ? { display: 'none' } : undefined}
              >
                {org.display_name || org.name} — shared with the whole organization
              </MenuItem>
            )
          })}
          {filteredOwnableOrgs.length === 0 && ownerFilter.trim() !== '' && (
            <MenuItem disabled>No organizations match &quot;{ownerFilter}&quot;</MenuItem>
          )}
        </Select>
      </FormControl>
      )}
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
        {ownerLocked
          ? 'Replacing the credentials on this subscription. The owner cannot be changed — disconnect it and connect a new one to move it.'
          : ownerType === 'org'
            ? `Anyone in ${orgLabel(selectedOrgId)} whose session has no personal Claude subscription will run on this one, and it will be billed to the Claude account you authenticate below.`
            : ''}
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
    <Dialog
      open={tokenDialogOpen}
      onClose={() => setTokenDialogOpen(false)}
      maxWidth="md"
      fullWidth
      // Full screen on phones: a dialog inset inside a 390px viewport leaves the
      // method cards no room to breathe.
      // Phones get the whole screen: an inset dialog inside a ~390px viewport
      // leaves the method cards no room. MUI's own pattern — an sx override with
      // an xs key would have applied at every width, since xs is the base.
      fullScreen={dialogFullScreen}
    >
      <DialogTitle sx={{ pb: 0.5 }}>
        {ownerLocked ? 'Update Claude Subscription' : 'Connect Claude Subscription'}
      </DialogTitle>
      <DialogContent sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2.5 }}>
        {/* The three methods differ in ways that matter after you pick one —
            how long the credential lives, what you need installed, and whether
            Helix can tell you whose subscription it is. The tiles state those
            up front instead of hiding them behind a chosen tab. */}
        <ClaudeConnectMethodPicker
          value={connectMethod}
          onChange={(next) => {
            setConnectMethod(next)
            setSubmitError(null)
          }}
        />

        {ownerPicker}

        {connectMethod === 'oauth' ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
              <StepIndex n={1} />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Authorize Helix on your Claude account
                </Typography>
                <Button
                  variant="contained"
                  color="secondary"
                  onClick={handleStartOauth}
                  disabled={startingOauth}
                  sx={{ textTransform: 'none' }}
                >
                  {startingOauth ? 'Opening…' : oauthChallenge ? 'Reopen Claude' : 'Sign in with Claude'}
                </Button>
                {oauthChallenge && (
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                    Didn't open?{' '}
                    <Box
                      component="a"
                      href={oauthChallenge.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      sx={{ color: 'secondary.main' }}
                    >
                      Open the authorization page
                    </Box>
                  </Typography>
                )}
              </Box>
            </Box>
            <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
              <StepIndex n={2} />
              <Typography variant="body2">
                Approve access, then copy the code Anthropic shows you.
              </Typography>
            </Box>
            <TextField
              fullWidth
              label="Authorization code"
              placeholder="Paste the code from Anthropic here…"
              value={oauthCode}
              onChange={(e) => setOauthCode(e.target.value)}
              disabled={!oauthChallenge}
              variant="outlined"
              InputProps={{ sx: { fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' } }}
            />
          </Box>
        ) : connectMethod === 'credentials' ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
              <StepIndex n={1} />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Print your existing Claude credentials
                </Typography>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    bgcolor: 'action.hover',
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 1,
                    px: 1.5,
                    py: 0.75,
                  }}
                >
                  <Box
                    component="code"
                    sx={{ flex: 1, fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' }}
                  >
                    {CREDENTIALS_FILE_COMMAND}
                  </Box>
                  <IconButton
                    size="small"
                    onClick={() => {
                      navigator.clipboard.writeText(CREDENTIALS_FILE_COMMAND)
                      snackbar.success('Command copied')
                    }}
                    aria-label="Copy command"
                  >
                    <Copy size={14} />
                  </IconButton>
                </Box>
              </Box>
            </Box>
            <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
              <StepIndex n={2} />
              <Typography variant="body2">Paste the whole JSON output below.</Typography>
            </Box>
            <TextField
              autoFocus
              fullWidth
              multiline
              minRows={4}
              label="Claude credentials JSON"
              placeholder='{"claudeAiOauth": {"accessToken": "…", "refreshToken": "…"}}'
              value={credentialsValue}
              onChange={(e) => setCredentialsValue(e.target.value)}
              variant="outlined"
              InputProps={{ sx: { fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' } }}
            />
          </Box>
        ) : (
        <>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
            <StepIndex n={1} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" sx={{ mb: 1 }}>
                Run this command in your terminal
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 1,
                  bgcolor: 'action.hover',
                  border: '1px solid',
                  borderColor: 'divider',
                  borderRadius: 1,
                  px: 1.5,
                  py: 0.75,
                }}
              >
                <Box
                  component="code"
                  sx={{
                    flex: 1,
                    fontFamily: APP_MONO_FONT_FAMILY,
                    fontSize: '0.8125rem',
                  }}
                >
                  {SETUP_TOKEN_COMMAND}
                </Box>
                <IconButton size="small" onClick={handleCopyCommand} aria-label="Copy command">
                  <Copy size={14} />
                </IconButton>
              </Box>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
            <StepIndex n={2} />
            <Typography variant="body2">
              Complete the authentication in your browser when prompted.
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
            <StepIndex n={3} />
            <Typography variant="body2">
              Copy the token that appears and paste it below.
            </Typography>
          </Box>
        </Box>

        <TextField
          autoFocus
          fullWidth
          type="password"
          label="Claude Code setup token"
          placeholder="Paste your Claude Code setup token here…"
          value={tokenValue}
          onChange={(e) => setTokenValue(e.target.value)}
          variant="outlined"
          InputProps={{
            sx: { fontFamily: APP_MONO_FONT_FAMILY, letterSpacing: '0.04em' },
          }}
        />
        </>
        )}

        {tokenValidationError && (
          <Alert severity="error">{tokenValidationError}</Alert>
        )}

        {submitError && (
          <Alert severity="error">{submitError}</Alert>
        )}

        <Typography variant="caption" color="text.secondary">
          To revoke this token later, visit{' '}
          <Box
            component="a"
            href="https://claude.ai/settings/claude-code"
            target="_blank"
            rel="noopener noreferrer"
            sx={{ color: 'secondary.main' }}
          >
            claude.ai/settings/claude-code
          </Box>
        </Typography>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={() => setTokenDialogOpen(false)} sx={{ textTransform: 'none' }}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmitToken}
          variant="contained"
          color="secondary"
          disabled={
            submitting ||
            !!tokenValidationError ||
            (connectMethod === 'oauth'
              ? !oauthChallenge || !oauthCode.trim()
              : connectMethod === 'credentials'
                ? !credentialsValue.trim()
                : !tokenValue.trim())
          }
          sx={{ textTransform: 'none' }}
        >
          {submitting ? <><CircularProgress size={14} sx={{ mr: 0.5 }} /> Connecting…</> : 'Connect'}
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
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={() => setDisconnectDialogOpen(false)} sx={{ textTransform: 'none' }}>Cancel</Button>
        <Button
          onClick={handleConfirmDelete}
          color="error"
          variant="contained"
          disabled={disconnectMutation.isPending}
          sx={{ textTransform: 'none' }}
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
        <Box
          sx={embedded ? {} : { mt: 2, backgroundColor: lightTheme.panelColor, p: 2, borderRadius: 2 }}
        >
            <Box
              sx={{
                display: 'flex',
                // A phone has no room for prose beside the button; side by side
                // the copy collapses into a ragged four-word column.
                flexDirection: { xs: 'column', sm: 'row' },
                justifyContent: 'space-between',
                alignItems: { xs: 'stretch', sm: 'center' },
                gap: { xs: 1.5, sm: 2 },
                mb: 2,
              }}
            >
              <Box>
                {!embedded && <Typography variant="h6">Claude Code Subscription</Typography>}
                <Typography variant="body2" color="text.secondary">
                  {!embedded &&
                    'Connect your Claude subscription to use Claude Code as the coding agent in Helix desktop sessions.'}
                  {ownableOrgs.length > 0 &&
                    (embedded
                      ? 'You can also connect one for an organization you own, as a shared fallback for members who have not connected their own.'
                      : ' You can also connect one for an organization you own, as a shared fallback for members who have not connected their own.')}
                </Typography>
              </Box>
              {/* With no orgs you own there is exactly one subscription you can
                  hold, and the card's "Update Token" is how you change it. */}
              {(!hasSubscription || ownableOrgs.length > 0) && (
                <Button
                  variant="outlined"
                  color="secondary"
                  size="small"
                  onClick={handleOpenTokenDialog}
                  sx={{ flexShrink: 0, textTransform: 'none', whiteSpace: 'nowrap' }}
                >
                  {hasSubscription ? 'Add subscription' : 'Connect subscription'}
                </Button>
              )}
            </Box>

            {isLoading ? (
              <Typography variant="body2" color="text.secondary">Loading...</Typography>
            ) : hasSubscription ? (
              subscriptions.map((sub) => {
                const subIsSetupToken = sub.credential_type === 'setup_token'
                // Background refresh keeps OAuth tokens alive; an expiry in the
                // past is normal and self-heals. Only the server's status says
                // whether this credential actually needs the user.
                const subIsBroken = sub.status !== 'active'
                // Same identity line as everywhere else, with the email hidden
                // until clicked — this pill is the most screenshotted of the lot.
                const subIdentity = (
                  <SubscriptionIdentity
                    email={sub.account_email}
                    fallback={sub.account_display_name || formatClaudeOrganizationRef(sub.claude_organization_id)}
                    detail={formatClaudeAccountDetail({
                      plan: sub.subscription_type,
                      tier: sub.rate_limit_tier,
                    })}
                    ariaLabel="Claude account email"
                    showPrefix
                  />
                )
                return (
                  <Box
                    key={sub.id}
                    sx={{
                      p: 2,
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: subIsBroken ? 'error.main' : 'divider',
                      display: 'flex',
                      flexDirection: { xs: 'column', sm: 'row' },
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
                        <Chip
                          {...(subIsBroken ? { icon: <CircleAlert size={16} /> } : {})}
                          label={subIsBroken ? 'Needs re-authentication' : 'Connected'}
                          color={subIsBroken ? 'error' : 'success'}
                          size="small"
                        />
                        {subIdentity && (
                          <Chip label={subIdentity} size="small" variant="outlined" />
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
                      </Box>
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
                    <Box sx={{ display: 'flex', gap: 0.75, alignItems: 'center', flexShrink: 0 }}>
                      <Button
                        variant={subIsBroken ? 'contained' : 'outlined'}
                        color={subIsBroken ? 'warning' : 'secondary'}
                        size="small"
                        onClick={() => handleOpenTokenDialogFor(sub)}
                        sx={{ textTransform: 'none', whiteSpace: 'nowrap' }}
                      >
                        {subIsBroken ? 'Re-authenticate' : 'Update token'}
                      </Button>
                      <Tooltip title="Disconnect subscription">
                        <IconButton
                          color="error"
                          aria-label="Disconnect subscription"
                          onClick={() => handleDeleteClick(sub.id)}
                          sx={{ width: 30, height: 30 }}
                        >
                          <Trash2 size={18} />
                        </IconButton>
                      </Tooltip>
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
        </Box>

        {tokenDialog}
        {disconnectDialog}
      </>
    )
  }

  // --- Button variant ---
  if (variant === 'button') {
    const connectButtonSx = { textTransform: 'none' } as const
    return (
      <>
        {hasSubscription ? (
          isExpired && !isSetupToken ? (
            <Button
              size="small"
              variant="contained"
              color="warning"
              onClick={handleOpenTokenDialog}
              startIcon={<CircleAlert size={16} />}
              sx={connectButtonSx}
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
              sx={connectButtonSx}
            >
              Disconnect
            </Button>
          )
        ) : (
          <Button
            size="small"
            variant="outlined"
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
            <Box component={CircleCheck} size={18} sx={{ color: 'success.main' }} />
            <Typography variant="body2" color="success.main">
              Claude subscription connected {isSetupToken ? '(setup token)' : ''}
            </Typography>
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
