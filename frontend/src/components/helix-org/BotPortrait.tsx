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

export const botPortraitForId = (botId: string): string => {
  let hash = 2166136261
  for (let index = 0; index < botId.length; index += 1) {
    hash = Math.imul(hash ^ botId.charCodeAt(index), 16777619)
  }
  return portraits[(hash >>> 0) % portraits.length]
}

interface BotPortraitProps {
  botId: string
  size?: number
  sx?: SxProps<Theme>
}

const BotPortrait: FC<BotPortraitProps> = ({ botId, size = 40, sx }) => (
  <Box
    component="img"
    src={botPortraitForId(botId)}
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

export default BotPortrait
