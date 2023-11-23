import React, { FC } from 'react'
import Box from '@mui/material/Box'
import prettyBytes from 'pretty-bytes'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ModelInstanceSummary from './ModelInstanceSummary'

import {
  IRunnerState,
} from '../../types'

export const RunnerSummary: FC<{
  runner: IRunnerState,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  runner,
  onViewSession,
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
        <Cell>
          <Typography variant="h6" sx={{mr: 2}}>{ runner.id }</Typography>
        </Cell>
        <Cell flexGrow={1} />
        <Cell>
          <Typography variant="caption" gutterBottom>{ Object.keys(runner.labels || {}).map(k => `${k}=${runner.labels[k]}`).join(', ') }</Typography>
        </Cell>
      </Row>
      <Row>
        <Cell>
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
              onViewSession={ onViewSession }
            />
          )
        })
      }
      <Box
        sx={{
          height: '100px',
          maxHeight: '100px',
          overflowY: 'auto',
        }}
      >
        <Typography component="ul" variant="caption" gutterBottom>
          {
            runner.scheduling_decisions.map((decision, i) => {
              return (
                <li key={ i }>{ decision }</li>
              )
              
            })
          }
        </Typography>
      </Box>
    </Box>
  )
}

export default RunnerSummary