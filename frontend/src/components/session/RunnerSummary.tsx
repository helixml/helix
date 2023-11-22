import React, { FC } from 'react'
import Box from '@mui/material/Box'
import prettyBytes from 'pretty-bytes'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import VisibilityIcon from '@mui/icons-material/Visibility'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ModelInstanceSummary from './ModelInstanceSummary'

import {
  IRunnerState,
} from '../../types'

import {
  getHeadline,
  getSummaryCaption,
  getTiming,
} from '../../utils/session'

export const RunnerSummary: FC<{
  runner: IRunnerState,
}> = ({
  runner,
}) => {
  const using_memory = runner.total_memory - runner.free_memory
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
      <Row>
        <Cell flexGrow={0}>
          <Typography variant="subtitle1" sx={{mr: 2}}>using { prettyBytes(using_memory) } of { prettyBytes(runner.total_memory) }</Typography>
        </Cell>
        <Cell flexGrow={1}>
          <LinearProgress
            variant="determinate"
            value={100 * using_memory / runner.total_memory}
            color="primary"
          />
        </Cell>
      </Row>
      {
        runner.model_instances.map(modelInstance => {
          return (
            <ModelInstanceSummary
              key={ modelInstance.id }
              modelInstance={ modelInstance }
            />
          )
        })
      }
    </Box>
  )
}

export default RunnerSummary