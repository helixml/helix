import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import SessionBadge from './SessionBadge'

import {
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
} from '../../types'

export const SessionBadgeKey: FC = () => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
      }}
    >
      <SessionBadge
        modelName="image"
        mode={ SESSION_MODE_INFERENCE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Image Inference
      </Typography>

      <SessionBadge
        modelName="image"
        mode={ SESSION_MODE_FINETUNE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Image Finetune
      </Typography>

      <SessionBadge
        modelName="text"
        mode={ SESSION_MODE_INFERENCE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Text Inference
      </Typography>

      <SessionBadge
        modelName="text"
        mode={ SESSION_MODE_FINETUNE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Text Finetune
      </Typography>

    </Box>
  )
}

export default SessionBadgeKey