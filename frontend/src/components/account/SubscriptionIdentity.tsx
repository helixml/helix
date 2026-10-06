import { FC } from 'react'
import Box from '@mui/material/Box'

import RedactedText from '../widgets/RedactedText'

interface Props {
  /** The account email, hidden until clicked. */
  email?: string | null
  /**
   * What to show when there is no email — a display name, a verified org
   * reference, or the subscription's own label. Not redacted: these are not
   * personal identifiers.
   */
  fallback?: string | null
  /** "Max · 20x" or "Pro" — never sensitive, always shown. */
  detail?: string | null
  ariaLabel?: string
  /**
   * Prefix the identity with "Authenticated as". Reads as a sentence next to a
   * redacted email, and makes clear the row is naming the credential's account
   * rather than the Helix user looking at it.
   */
  showPrefix?: boolean
}

/**
 * The one place a subscription's identity is rendered.
 *
 * The email is hidden by default because these rows sit on screens people
 * share and screenshot, while the plan and tier are not personal and stay
 * visible — you can still tell which subscription would be spent without
 * exposing whose it is.
 */
const SubscriptionIdentity: FC<Props> = ({
  email,
  fallback,
  detail,
  ariaLabel = 'Account email',
  showPrefix = false,
}) => {
  const trimmedEmail = email?.trim()
  const lead = trimmedEmail ? (
    <RedactedText value={trimmedEmail} ariaLabel={ariaLabel} />
  ) : (
    fallback?.trim() || null
  )

  if (!lead && !detail) return null

  return (
    <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
      {showPrefix && lead ? <Box component="span">Authenticated as</Box> : null}
      {lead}
      {lead && detail ? <Box component="span">·</Box> : null}
      {detail ? <Box component="span">{detail}</Box> : null}
    </Box>
  )
}

export default SubscriptionIdentity
