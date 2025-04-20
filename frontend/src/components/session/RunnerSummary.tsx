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
  // Calculate memory values - we get total_memory and free_memory from the API
  const actual_memory = runner.total_memory - runner.free_memory
  
  // Get allocated memory from the API if available, otherwise fall back to actual memory
  const allocated_memory = runner.allocated_memory || actual_memory

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
          <Typography variant="subtitle1" sx={{mr: 2}}>
            Actual: { prettyBytes(actual_memory) } / Allocated: { prettyBytes(allocated_memory) } / Total: { prettyBytes(runner.total_memory) }
          </Typography>
        </Cell>
        <Cell flexGrow={1}>
          {/* Dual progress bar implementation */}
          <Box sx={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
            {/* Outer bar (allocated memory) */}
            <LinearProgress
              variant="determinate"
              value={100 * allocated_memory / runner.total_memory}
              color="primary"
              sx={{ 
                width: '100%',
                height: 10,
                borderRadius: 1,
                backgroundColor: '#e0e0e0' 
              }}
            />
            {/* Inner bar (actual memory) */}
            <LinearProgress
              variant="determinate"
              value={100 * actual_memory / runner.total_memory}
              color="secondary"
              sx={{ 
                position: 'absolute', 
                width: '100%', 
                height: 5,
                borderRadius: 1,
              }}
            />
          </Box>
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