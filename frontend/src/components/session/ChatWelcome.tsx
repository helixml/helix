import { FC, ReactNode } from 'react'
import { Box, Typography } from '@mui/material'

import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'

/**
 * The "nothing has been said yet" screen: a heading, a composer under it, and
 * a lot of quiet around both.
 *
 * Extracted from Home so the embedded chat can be the same screen rather than a
 * lookalike. Two implementations of this drift — one gets the phone behaviour
 * and the other does not, one gets the font and the other does not — and the
 * difference shows up on a customer's screen rather than ours.
 *
 * Pure layout. It owns no state and knows nothing about sessions, projects or
 * sending: the caller passes the composer as `children` and anything that
 * belongs under it as `footer`.
 */
const ChatWelcome: FC<{
  heading: string
  children: ReactNode
  footer?: ReactNode
  /**
   * Sits under the heading, above the composer. For a line of context the
   * heading alone cannot carry.
   */
  subheading?: string
}> = ({ heading, children, footer, subheading }) => {
  const isPhone = useIsPhone()
  const lightTheme = useLightTheme()

  return (
    <Box
      sx={{
        height: '100%',
        minHeight: 0,
        display: 'flex',
        // A phone fills the screen and works top-down; a wide screen keeps
        // the centred card.
        alignItems: isPhone ? 'stretch' : 'center',
        justifyContent: 'center',
        px: { xs: 2, sm: 3 },
        // The shell already carries the safe-area inset.
        pb: isPhone ? 1 : { xs: 4, md: 12 },
        pt: isPhone ? 1 : 0,
        backgroundColor: lightTheme.isLight ? '#f7f7f8' : '#080808',
        fontFamily: WELCOME_FONT_FAMILY,
        '& .MuiTypography-root, & .MuiButton-root': { fontFamily: 'inherit' },
      }}
    >
      <Box
        sx={{
          width: '100%',
          maxWidth: 768,
          ...(isPhone && { display: 'flex', flexDirection: 'column', minHeight: 0 }),
        }}
      >
        {!isPhone && (
          <>
            <Typography
              component="h1"
              sx={{
                mb: subheading ? 1 : 3.5,
                color: 'text.primary',
                fontSize: { xs: '1.65rem', sm: '1.9rem' },
                fontWeight: 560,
                lineHeight: 1.2,
                letterSpacing: '-0.025em',
                textAlign: 'center',
              }}
            >
              {heading}
            </Typography>
            {subheading && (
              <Typography
                sx={{
                  mb: 3.5,
                  color: 'text.secondary',
                  fontSize: '0.95rem',
                  lineHeight: 1.5,
                  textAlign: 'center',
                }}
              >
                {subheading}
              </Typography>
            )}
          </>
        )}

        {children}

        {footer && (
          <Box sx={isPhone ? { order: -1, mb: 0.5, flexShrink: 0 } : { mt: 1, px: 2 }}>
            {footer}
          </Box>
        )}
      </Box>
    </Box>
  )
}

/** The system UI stack, matching the rest of the new-chat screen. */
export const WELCOME_FONT_FAMILY =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif'

export default ChatWelcome
