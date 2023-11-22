import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import SessionBadge from './SessionBadge'

import {
  ISession,
} from '../../types'

import {
  getHeadline,
  getSummary,
} from '../../utils/session'

export const SessionSummaryRow: FC<{
  session: ISession,
}> = ({
  session,
}) => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        p: 1,
        border: '1px solid #ccc'
      }}
    >
      <Box
        sx={{
          mr: 2,
        }}
      >
        <SessionBadge
          session={ session }
        />
      </Box>
      <Box
        sx={{
          mr: 2,
        }}
      >
        <Typography
          sx={{lineHeight: 1}}
          variant="body2"
        >
          { getHeadline(session) }
        </Typography>
        <Typography
          variant="caption"
        >
          { getSummary(session) }
        </Typography>
      </Box>
    </Box>
  )
}

export default SessionSummaryRow