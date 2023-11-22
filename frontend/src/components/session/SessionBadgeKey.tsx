import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import SessionBadge from './SessionBadge'

import {
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
} from '../../types'

export const SessionBadgeKey: FC<{
  
}> = ({
  
}) => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
      }}
    >
      <SessionBadge
        type={ SESSION_TYPE_IMAGE }
        mode={ SESSION_MODE_INFERENCE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        SDXL Inference
      </Typography>

      <SessionBadge
        type={ SESSION_TYPE_IMAGE }
        mode={ SESSION_MODE_FINETUNE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        SDXL Finetune
      </Typography>

      <SessionBadge
        type={ SESSION_TYPE_TEXT }
        mode={ SESSION_MODE_INFERENCE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Mistral Inference
      </Typography>

      <SessionBadge
        type={ SESSION_TYPE_TEXT }
        mode={ SESSION_MODE_FINETUNE }
      />
      <Typography
        sx={{
          ml: 1,
          mr: 2,
        }}
      >
        Mistral Finetune
      </Typography>

    </Box>
  )
}

export default SessionBadgeKey