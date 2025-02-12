import Box from '@mui/material/Box'
import LinearProgress from '@mui/material/LinearProgress'
import Typography from '@mui/material/Typography'
import { FC } from 'react'
import { prettyBytes } from '../../utils/format'
import Cell from '../widgets/Cell'
import Row from '../widgets/Row'
import ModelInstanceSummary from './ModelInstanceSummary'

import {
  IRunnerStatus
} from '../../types'

export const RunnerSummary: FC<{
  runner: IRunnerStatus,
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
          <Typography variant="h6" sx={{mr: 2}}>{ runner.id } { runner.version }</Typography>
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
        runner.slots
          ?.sort((a, b) => a.id.localeCompare(b.id))
          .map(slot => {
          return (
            <ModelInstanceSummary
              key={ slot.id }
              slot={ slot }
              onViewSession={ onViewSession }
            />
          )
        })
      }
    </Box>
  )
}

export default RunnerSummary