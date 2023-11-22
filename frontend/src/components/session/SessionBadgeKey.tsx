import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import SessionBadge from './SessionBadge'

import {
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  MODEL_NAME_SDXL,
  MODEL_NAME_MISTRAL,
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
        modelName={ MODEL_NAME_SDXL }
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
        modelName={ MODEL_NAME_SDXL }
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
        modelName={ MODEL_NAME_MISTRAL }
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
        modelName={ MODEL_NAME_MISTRAL }
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