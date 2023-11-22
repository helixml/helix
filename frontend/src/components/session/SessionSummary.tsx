import React, { FC } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import VisibilityIcon from '@mui/icons-material/Visibility'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'

import {
  ISession,
} from '../../types'

import {
  getSessionHeadline,
  getSummaryCaption,
} from '../../utils/session'

export const SessionSummary: FC<{
  session: ISession,
}> = ({
  session,
}) => {
  return (
    <Box
      sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        p: 1,
        border: '1px dashed #ccc',
        borderRadius: '8px',
        mb: 2,
      }}
    >
      <Box
        sx={{
          flexGrow: 0,
        }}
      >
        <SessionBadge
          modelName={ session.model_name }
          mode={ session.mode }
        />
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          ml: 1,
          mr: 1,
        }}
      >
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { getSessionHeadline(session) }
        </Typography>
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { getSummaryCaption(session) }
        </Typography>
      </Box>
      <Box
        sx={{
          flexGrow: 0,
        }}
      >
        <JsonWindowLink
          data={ session }
        >
          <IconButton color="primary">
            <VisibilityIcon />
          </IconButton>
        </JsonWindowLink>
      </Box>
    </Box>
  )
}

export default SessionSummary