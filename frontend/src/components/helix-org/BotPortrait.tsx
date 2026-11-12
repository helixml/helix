import { FC } from 'react'
import Box from '@mui/material/Box'
import type { SxProps, Theme } from '@mui/material/styles'

import cyan from '../../assets/bots/cyan.webp'
import indigo from '../../assets/bots/indigo.webp'
import lavender from '../../assets/bots/lavender.webp'
import magenta from '../../assets/bots/magenta.webp'
import pearl from '../../assets/bots/pearl.webp'
import teal from '../../assets/bots/teal.webp'

const portraits = [lavender, cyan, magenta, indigo, teal, pearl] as const

interface BotPortraitProps {
  portraitIndex?: number
  size?: number
  sx?: SxProps<Theme>
}

const BotPortrait: FC<BotPortraitProps> = ({ portraitIndex, size = 40, sx }) => {
  if (portraitIndex === undefined || !Number.isInteger(portraitIndex) || portraitIndex < 0 || portraitIndex >= portraits.length) {
    return null
  }
  return (
    <Box
      component="img"
      src={portraits[portraitIndex]}
      alt=""
      aria-hidden="true"
      sx={[
        {
          width: size,
          height: size,
          display: 'block',
          flexShrink: 0,
          objectFit: 'contain',
        },
        ...(sx ? (Array.isArray(sx) ? sx : [sx]) : []),
      ]}
    />
  )
}

export default BotPortrait
