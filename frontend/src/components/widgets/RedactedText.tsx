import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'

import { APP_MONO_FONT_FAMILY } from '../../styles/typography'

const PLACEHOLDER_ALPHABET = 'abcdefghjkmnpqrstuvwxyz23456789'

/**
 * A stable nonsense string of the same shape as the input.
 *
 * Separators are kept so a redacted email still reads as an email, which is
 * what makes the blur look like content rather than a smudge. Everything else
 * is derived from an FNV-1a hash of the value, so the same address always
 * redacts to the same placeholder — no flicker between renders — while the
 * placeholder tells you nothing about the original.
 */
function redactedPlaceholder(value: string): string {
  let state = 0x811c9dc5
  for (let i = 0; i < value.length; i += 1) {
    state ^= value.charCodeAt(i)
    state = Math.imul(state, 0x01000193)
  }
  const nextChar = () => {
    state = Math.imul(state ^ (state >>> 13), 0x85ebca6b)
    state = Math.imul(state ^ (state >>> 16), 0xc2b2ae35)
    return PLACEHOLDER_ALPHABET[Math.abs(state) % PLACEHOLDER_ALPHABET.length] ?? 'x'
  }
  return Array.from(value, (char) =>
    char === '@' || char === '.' || char === '-' || char === '_' ? char : nextChar(),
  ).join('')
}

interface Props {
  value?: string | null
  /** Describes what is hidden, for screen readers. */
  ariaLabel?: string
  className?: string
}

/**
 * Hides an identifier until asked for, for screens someone might be sharing.
 *
 * The real value is never rendered while hidden — a CSS blur alone would leave
 * it selectable, copyable and visible in the DOM, which defeats the point on a
 * shared screen or a screenshot. Click to toggle.
 */
const RedactedText: FC<Props> = ({ value, ariaLabel = 'Hidden value', className }) => {
  const [revealed, setRevealed] = useState(false)
  const trimmed = value?.trim()
  const placeholder = useMemo(() => (trimmed ? redactedPlaceholder(trimmed) : ''), [trimmed])

  if (!trimmed) return null

  return (
    <Tooltip title={revealed ? 'Click to hide' : 'Click to reveal'}>
      <Box
        component="button"
        type="button"
        aria-label={ariaLabel}
        onClick={(e: React.MouseEvent) => {
          // These sit inside clickable rows and chips.
          e.stopPropagation()
          e.preventDefault()
          setRevealed((current) => !current)
        }}
        className={className}
        sx={{
          m: 0,
          p: 0,
          border: 0,
          background: 'transparent',
          color: 'inherit',
          font: 'inherit',
          fontFamily: APP_MONO_FONT_FAMILY,
          cursor: 'pointer',
          maxWidth: '100%',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          verticalAlign: 'bottom',
          ...(revealed
            ? {}
            : { filter: 'blur(3px)', userSelect: 'none' }),
          '&:focus-visible': {
            outline: '2px solid',
            outlineColor: 'secondary.main',
            outlineOffset: 2,
          },
        }}
      >
        {revealed ? trimmed : placeholder}
      </Box>
    </Tooltip>
  )
}

export default RedactedText
