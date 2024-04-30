import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import {
  ISessionType,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
} from '../../types'

import {
  COLORS,
} from '../../config'

const SessionTypeTabs: FC<{
  type: ISessionType,
  onSetType: (type: ISessionType) => void,
}> = ({
  type,
  onSetType,
}) => {
  return (
    <Box
      sx={{
        width: '100%',
        display: 'flex',
        justifyContent: 'center',
      }}
    >
      <Box sx={{ display: 'flex', width: '100%' }}>
        <Box
          sx={{
            width: '50%',
            textAlign: 'center',
            cursor: 'pointer',
            '&:after': {
              content: '""',
              display: 'block',
              height: '2px',
              backgroundColor: type === SESSION_TYPE_IMAGE ? COLORS[SESSION_TYPE_IMAGE] : 'rgba(255, 255, 255, 0.2)', // Light white with transparency if Text is selected
              marginTop: '0.25rem',
            }
          }}
          onClick={() => onSetType(SESSION_TYPE_IMAGE)}
        >
          <Typography
            variant="subtitle1"
            sx={{
              fontSize: "medium",
              fontWeight: 800,
              color: type === SESSION_TYPE_IMAGE ? COLORS[SESSION_TYPE_IMAGE] : 'rgba(255, 255, 255, 0.2)',
              marginBottom: '10px',
            }}
          >
            Images
          </Typography>
        </Box>
        <Box
          sx={{
            width: '50%',
            textAlign: 'center',
            cursor: 'pointer',
            '&:after': {
              content: '""',
              display: 'block',
              height: '2px',
              backgroundColor: type === SESSION_TYPE_TEXT ? COLORS[SESSION_TYPE_TEXT] : 'rgba(255, 255, 255, 0.2)',
            }
          }}
          onClick={() => onSetType(SESSION_TYPE_TEXT)}
        >
          <Typography
            variant="subtitle1"
            sx={{
              fontSize: "medium",
              fontWeight: 800,
              color: type === SESSION_TYPE_TEXT ? COLORS[SESSION_TYPE_TEXT] : 'rgba(255, 255, 255, 0.2)',
              marginBottom: '10px',
            }}
          >
            Text
          </Typography>
        </Box>
      </Box>
    </Box>
  )
}

export default SessionTypeTabs
