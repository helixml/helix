import React, { FC } from 'react'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import VisibilityIcon from '@mui/icons-material/Visibility'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  IGlobalSchedulingDecision,
} from '../../types'

import {
  shortID,
} from '../../utils/session'

export const SchedulingDecisionSummary: FC<{
  decision: IGlobalSchedulingDecision,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  decision,
  onViewSession,
}) => {
  return (
    <Row>
      <Cell
        sx={{
          width: '30px'
        }}
      >
        <SessionBadge
          modelName={ decision.model_name }
          mode={ decision.mode }
          reverse
        />
      </Cell>
      <Cell
        sx={{
          width: '55px'
        }}
      >
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { decision.created.split('T')[1].split('.')[0] }
        </Typography>
      </Cell>
      <Cell
        sx={{
          width: '130px'
        }}
      >
        <JsonWindowLink
          data={ decision }
        >
          <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', textDecoration: 'underline' }}> 
            { shortID(decision.session_id) } : { shortID(decision.interaction_id) }
          </Typography>
        </JsonWindowLink>
      </Cell>
      <Cell flexGrow={ 1 }>
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          -&gt; { decision.runner_id }
        </Typography>
      </Cell>
      <Cell>
        <IconButton
          color="primary"
          onClick={ () => {
            onViewSession(decision.session_id)
          }}
        >
          <VisibilityIcon />
        </IconButton>
      </Cell>
    </Row>
  )
}

export default SchedulingDecisionSummary