import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import { TriangleAlert } from 'lucide-react'

import { useClaudeSubscriptions } from './ClaudeSubscriptionConnect'
import { getClaudeLoginExpiry } from './claudeSubscriptionUtils'

interface Props {
  /** Opens the account settings dialog, where the subscription is reconnected. */
  onReconnect: () => void
}

/**
 * Warns that a Claude login is about to lapse, or already has.
 *
 * A Claude sign-in is good for about nine days and refreshing does not extend
 * it, so every OAuth subscription eventually needs the user to sign in again.
 * Without a warning that arrives before the deadline, the first anyone knows is
 * an agent turn failing — which is what used to happen.
 *
 * Deliberately silent until the last day: `getClaudeLoginExpiry` returns null
 * while there is more than a day left, and for setup tokens, which have no
 * refresh token and so no deadline to read.
 */
const ClaudeExpiryBanner: FC<Props> = ({ onReconnect }) => {
  const { data: subscriptions } = useClaudeSubscriptions()
  const subscription = subscriptions?.find((sub) => sub.owner_type === 'user')
  const expiry = getClaudeLoginExpiry(subscription?.refresh_token_expires_at)

  if (!subscription || !expiry) return null

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1.25,
        px: 2,
        py: 1.25,
        borderBottom: '1px solid',
        borderColor: 'divider',
        bgcolor: expiry.isExpired ? 'rgba(239,68,68,0.10)' : 'rgba(245,158,11,0.10)',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          flexShrink: 0,
          color: expiry.isExpired ? 'error.main' : 'warning.main',
        }}
      >
        <TriangleAlert size={16} />
      </Box>
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.78rem' }}>
          {expiry.isExpired ? 'Claude sign-in expired' : 'Claude sign-in expires today'}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.7rem' }}>
          {expiry.label}
          {expiry.isExpired
            ? ' — agents using this subscription will fail until you sign in again.'
            : ' — sign in again to avoid interrupting running agents.'}
        </Typography>
      </Box>
      <Button
        size="small"
        variant="contained"
        color={expiry.isExpired ? 'error' : 'warning'}
        onClick={onReconnect}
        sx={{ textTransform: 'none', whiteSpace: 'nowrap', flexShrink: 0 }}
      >
        Sign in
      </Button>
    </Box>
  )
}

export default ClaudeExpiryBanner
