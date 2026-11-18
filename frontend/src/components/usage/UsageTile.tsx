// Tile is the compact labelled stat used across the session Usage tab.

import { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import useLightTheme from '../../hooks/useLightTheme'

export const SECTION_LABEL_SX = { fontSize: '0.65rem', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'text.secondary' } as const

export const Tile: FC<{ label: string; value: string; note?: string }> = ({ label, value, note }) => {
  const lightTheme = useLightTheme()
  return (
    <Box
      sx={{
        p: 1.25,
        borderRadius: 1.5,
        border: '1px solid',
        borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.06)',
        background: lightTheme.isLight ? 'rgba(0,0,0,0.015)' : 'rgba(255,255,255,0.02)',
        minWidth: 0,
      }}
    >
      <Typography variant="caption" sx={SECTION_LABEL_SX}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontSize: '1rem', fontWeight: 600, fontVariantNumeric: 'tabular-nums', lineHeight: 1.3 }} noWrap>
        {value}
      </Typography>
      {note && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontVariantNumeric: 'tabular-nums' }} noWrap>
          {note}
        </Typography>
      )}
    </Box>
  )
}
