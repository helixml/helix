import Box from '@mui/material/Box'
import LinearProgress from '@mui/material/LinearProgress'
import Typography from '@mui/material/Typography'
import { FC } from 'react'
import { prettyBytes } from '../../utils/format'
import Cell from '../widgets/Cell'
import Row from '../widgets/Row'
import ModelInstanceSummary from './ModelInstanceSummary'
import Paper from '@mui/material/Paper'
import Divider from '@mui/material/Divider'
import Chip from '@mui/material/Chip'

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
    <Paper
      elevation={2}
      sx={{
        width: '100%',
        minWidth: 500,
        minHeight: 150,
        mb: 3,
        backgroundColor: 'background.paper',
        borderLeft: '4px solid',
        borderColor: 'primary.main',
        borderRadius: 0,
        overflow: 'hidden',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: 4,
        }
      }}
    >
      <Box sx={{ p: 2 }}>
        <Row>
          <Cell>
            <Typography 
              variant="h6" 
              fontWeight="bold"
              sx={{ 
                mr: 2,
                display: 'flex',
                alignItems: 'center',
              }}
            >
              { runner.id } 
              <Typography 
                variant="caption" 
                sx={{ 
                  ml: 1,
                  px: 1,
                  py: 0.5,
                  borderRadius: 0,
                  backgroundColor: 'grey.100',
                  color: 'grey.800',
                }}
              >
                { runner.version }
              </Typography>
            </Typography>
          </Cell>
          <Cell flexGrow={1} />
          <Cell>
            {Object.keys(runner.labels || {}).map(k => (
              <Chip 
                key={k}
                size="small"
                label={`${k}=${runner.labels[k]}`} 
                sx={{ 
                  mr: 0.5, 
                  borderRadius: 0,
                  backgroundColor: 'background.default',
                }}
              />
            ))}
          </Cell>
        </Row>
        
        <Divider sx={{ my: 1.5 }} />
        
        <Row>
          <Cell>
            <Typography variant="subtitle2" sx={{ mr: 2, fontSize: '0.85rem' }}>
              <Box component="span" sx={{ fontWeight: 'bold', color: 'secondary.main' }}>Actual:</Box> { prettyBytes(actual_memory) } / 
              <Box component="span" sx={{ fontWeight: 'bold', color: 'primary.main', ml: 1 }}>Allocated:</Box> { prettyBytes(allocated_memory) } / 
              <Box component="span" sx={{ fontWeight: 'bold', ml: 1 }}>Total:</Box> { prettyBytes(runner.total_memory) }
            </Typography>
          </Cell>
          <Cell flexGrow={1}>
            <Box sx={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
              {/* Memory usage background */}
              <Box sx={{ 
                position: 'absolute',
                width: '100%',
                height: 12,
                backgroundColor: 'grey.100',
              }} />
              
              {/* Allocated memory bar */}
              <LinearProgress
                variant="determinate"
                value={100 * allocated_memory / runner.total_memory}
                color="primary"
                sx={{ 
                  width: '100%',
                  height: 12,
                  borderRadius: 0,
                  backgroundColor: 'transparent',
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 0,
                  }
                }}
              />
              
              {/* Actual memory bar */}
              <LinearProgress
                variant="determinate"
                value={100 * actual_memory / runner.total_memory}
                color="secondary"
                sx={{ 
                  position: 'absolute', 
                  width: '100%', 
                  height: 6,
                  top: 3,
                  borderRadius: 0,
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 0,
                  }
                }}
              />
            </Box>
          </Cell>
        </Row>
      </Box>
      
      {runner.slots && runner.slots.length > 0 && (
        <Box sx={{ 
          mt: 1,
          backgroundColor: 'background.default',
          pt: 1,
        }}>
          {runner.slots
            ?.sort((a, b) => a.id.localeCompare(b.id))
            .map(slot => (
              <ModelInstanceSummary
                key={slot.id}
                slot={slot}
                onViewSession={onViewSession}
              />
            ))
          }
        </Box>
      )}
      
      {(!runner.slots || runner.slots.length === 0) && (
        <Box sx={{ 
          backgroundColor: 'background.default',
          p: 2,
          textAlign: 'center',
        }}>
          <Typography variant="body2" color="text.secondary">
            No active model instances
          </Typography>
        </Box>
      )}
    </Paper>
  )
}

export default RunnerSummary