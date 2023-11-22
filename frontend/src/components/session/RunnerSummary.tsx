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
  IRunnerState,
} from '../../types'

import {
  getHeadline,
  getSummary,
  getTiming,
} from '../../utils/session'

export const RunnerSummary: FC<{
  runner: IRunnerState,
}> = ({
  runner,
}) => {
  return (
    <Box
      sx={{
        width: '100%',
        p: 1,
        border: '1px dashed #ccc',
        borderRadius: '8px',
        mb: 2,
      }}
    >
      <Row>
        <Cell flexGrow={0}>
          <Typography variant="h6" sx={{mr: 2}}>{ runner.id }</Typography>
        </Cell>
        <Cell flexGrow={1} />
        <Cell flexGrow={0}>
          <Typography variant="caption" gutterBottom>{ Object.keys(runner.labels || {}).map(k => `${k}=${runner.labels[k]}`).join(', ') }</Typography>
        </Cell>
      </Row>
    </Box>
  )
}

export default RunnerSummary