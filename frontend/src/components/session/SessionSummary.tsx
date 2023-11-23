import React, { FC } from 'react'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import VisibilityIcon from '@mui/icons-material/Visibility'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  ISessionSummary,
} from '../../types'

import {
  getSummaryCaption,
  getHeadline,
  shortID,
  getTiming,
} from '../../utils/session'

export const SessionSummary: FC<{
  session: ISessionSummary,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  session,
  onViewSession,
}) => {
  return (
    <Row
      sx={{
        mt: 1,
        mb: 1,
      }}
    >
      <Cell
        sx={{
          width: '30px'
        }}
      >
        <SessionBadge
          modelName={ session.model_name }
          mode={ session.mode }
        />
      </Cell>
      <Cell flexGrow={ 1 }>
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { getHeadline(session.model_name, session.mode) } : <JsonWindowLink data={ session }>{ shortID(session.session_id) }</JsonWindowLink> : { getTiming(session) }
        </Typography>
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', color: '#999' }}>
          { getSummaryCaption(session) }
        </Typography>
      </Cell>
      <Cell>
        <IconButton
          color="primary"
          onClick={ () => {
            onViewSession(session.session_id)
          }}
        >
          <VisibilityIcon />
        </IconButton>
      </Cell>
    </Row>
  )
}

export default SessionSummary