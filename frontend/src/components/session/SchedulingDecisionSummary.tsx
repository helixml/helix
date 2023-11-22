import React, { FC } from 'react'
import Box from '@mui/material/Box'
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
}> = ({
  decision,
}) => {
  console.dir(decision)
  return (
    <Row>
      <Cell
        sx={{
          width: '70px'
        }}
      >
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { decision.created.split('T')[1].split('.')[0] }
        </Typography>
      </Cell>
      <Cell
        sx={{
          width: '70px'
        }}
      >
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          { shortID(decision.session_id) }
        </Typography>
      </Cell>
      <Cell flexGrow={ 1 }>
        <Typography component="div" variant="caption" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          -&gt; { decision.runner_id }
        </Typography>
      </Cell>
      <Cell>
        <JsonWindowLink
          data={ decision }
        >
          <IconButton color="primary">
            <VisibilityIcon />
          </IconButton>
        </JsonWindowLink>
      </Cell>
    </Row>
  )
}

export default SchedulingDecisionSummary