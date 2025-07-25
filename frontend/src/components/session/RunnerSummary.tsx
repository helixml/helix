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
import Grid from '@mui/material/Grid'
import Tooltip from '@mui/material/Tooltip'
import CircularProgress from '@mui/material/CircularProgress'

import {
  TypesDashboardRunner
} from '../../api/api'

export const RunnerSummary: FC<{
  runner: TypesDashboardRunner,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  runner,
  onViewSession,
}) => {
  // Get memory values with proper fallbacks to ensure we have valid numbers
  const total_memory = runner.total_memory || 1  // Avoid division by zero
  const used_memory = typeof runner.used_memory === 'number' ? runner.used_memory : 0
  const allocated_memory = (!runner.slots || runner.slots.length === 0 || !runner.allocated_memory) 
    ? 0 
    : runner.allocated_memory
  
  // Calculate percentages with safeguards against NaN or Infinity
  const actualPercent = isFinite(Math.round((used_memory / total_memory) * 100)) 
    ? Math.round((used_memory / total_memory) * 100) 
    : 0
    
  const allocatedPercent = isFinite(Math.round((allocated_memory / total_memory) * 100))
    ? Math.round((allocated_memory / total_memory) * 100)
    : 0

  return (
    <Paper
      elevation={3}
      sx={{
        width: '100%',
        minWidth: 600,
        minHeight: 180,
        mb: 3,
        backgroundColor: 'rgba(30, 30, 32, 0.95)',
        borderLeft: '4px solid',
        borderColor: '#00c8ff',
        borderRadius: '3px',
        overflow: 'hidden',
        position: 'relative',
        backdropFilter: 'blur(10px)',
        boxShadow: '0 6px 14px -2px rgba(0, 0, 0, 0.2), 0 0 0 1px rgba(255, 255, 255, 0.05)',
        '&::before': {
          content: '""',
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          height: '100%',
          backgroundImage: 'linear-gradient(180deg, rgba(0, 200, 255, 0.08) 0%, rgba(0, 0, 0, 0) 30%)',
          pointerEvents: 'none',
        },
      }}
    >
      {/* Side glow effect */}
      <Box 
        sx={{ 
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: '4px',
          background: 'linear-gradient(180deg, #00c8ff 0%, rgba(0, 200, 255, 0.3) 100%)',
          boxShadow: '0 0 15px 1px rgba(0, 200, 255, 0.5)',
          opacity: 0.8,
          zIndex: 2,
        }} 
      />
      
      {/* Light reflection effect */}
      <Box 
        sx={{ 
          position: 'absolute',
          right: 0,
          top: 0,
          width: '40%', 
          height: '100%',
          background: 'linear-gradient(90deg, rgba(255,255,255,0) 0%, rgba(255,255,255,0.03) 100%)',
          pointerEvents: 'none',
          opacity: 0.5,
        }} 
      />
      
      <Box sx={{ p: 2.5 }}>
        <Grid container alignItems="center" spacing={2}>
          <Grid item xs>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <Typography 
                variant="h6" 
                fontWeight="600"
                sx={{ 
                  mr: 2,
                  color: '#fff',
                  letterSpacing: '0.5px',
                }}
              >
                { runner.id }
              </Typography>
              
              <Box 
                sx={{ 
                  display: 'inline-block',
                  px: 1.5,
                  py: 0.5,
                  backgroundColor: 'rgba(255, 255, 255, 0.07)',
                  border: '1px solid rgba(255, 255, 255, 0.1)',
                  borderRadius: '3px',
                  boxShadow: 'inset 0 1px 1px rgba(0, 0, 0, 0.1)',
                  backdropFilter: 'blur(5px)',
                }}
              >
                <Typography 
                  variant="caption" 
                  sx={{ 
                    fontFamily: 'monospace',
                    color: 'rgba(255, 255, 255, 0.7)',
                    fontWeight: 500,
                    letterSpacing: '0.5px',
                  }}
                >
                  { runner.version || 'unknown' }
                </Typography>
              </Box>
            </Box>
          </Grid>
          
          <Grid item>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              {Object.keys(runner.labels || {}).map(k => (
                <Chip 
                  key={k}
                  size="small"
                  label={`${k}=${runner.labels?.[k]}`} 
                  sx={{ 
                    mr: 0.5,
                    mb: 0.5,
                    borderRadius: '3px',
                    backgroundColor: 'rgba(0, 200, 255, 0.08)',
                    border: '1px solid rgba(0, 200, 255, 0.2)',
                    color: 'rgba(255, 255, 255, 0.85)',
                    '& .MuiChip-label': {
                      fontSize: '0.7rem',
                      px: 1.2,
                    }
                  }}
                />
              ))}
            </Box>
          </Grid>
        </Grid>
        
        <Divider sx={{ 
          my: 2, 
          borderColor: 'rgba(255, 255, 255, 0.06)',
          boxShadow: '0 1px 2px rgba(0, 0, 0, 0.1)', 
        }} />
        
        <Grid container spacing={2} alignItems="center">
          <Grid item xs={12} md={5}>
            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                <Typography 
                  variant="caption" 
                  sx={{ 
                    color: '#00c8ff', 
                    fontWeight: 600,
                  }}
                >
                  Actual: { prettyBytes(used_memory) } ({actualPercent}%)
                </Typography>
                <Typography 
                  variant="caption" 
                  sx={{ 
                    color: 'rgba(255, 255, 255, 0.5)',
                    fontWeight: 500,
                  }}
                >
                  Total: { prettyBytes(total_memory) }
                </Typography>
              </Box>
              <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                <Typography 
                  variant="caption" 
                  sx={{ 
                    color: '#7986cb', 
                    fontWeight: 600,
                  }}
                >
                  Allocated: { prettyBytes(allocated_memory) } ({allocatedPercent}%)
                </Typography>
              </Box>
            </Box>
          </Grid>
          
          <Grid item xs={12} md={7}>
            <Box sx={{ position: 'relative', display: 'flex', alignItems: 'center', height: 20 }}>
              {/* Memory usage background with shine effect */}
              <Box sx={{ 
                position: 'absolute',
                width: '100%',
                height: 12,
                backgroundColor: 'rgba(255, 255, 255, 0.03)',
                borderRadius: '4px',
                boxShadow: 'inset 0 1px 3px rgba(0,0,0,0.3)',
                overflow: 'hidden',
              }} />
              
              {/* Allocated memory bar */}
              <LinearProgress
                variant="determinate"
                value={100 * allocated_memory / total_memory}
                sx={{ 
                  width: '100%',
                  height: 12,
                  borderRadius: '4px',
                  backgroundColor: 'transparent',
                  '& .MuiLinearProgress-bar': {
                    background: 'linear-gradient(90deg, rgba(121,134,203,0.9) 0%, rgba(121,134,203,0.7) 100%)',
                    borderRadius: '4px',
                    transition: 'transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)',
                    boxShadow: '0 0 10px rgba(121,134,203,0.5)'
                  }
                }}
              />
              
              {/* Actual memory bar */}
              <LinearProgress
                variant="determinate"
                value={100 * used_memory / total_memory}
                sx={{ 
                  position: 'absolute', 
                  width: '100%', 
                  height: 6,
                  top: 3,
                  borderRadius: '4px',
                  backgroundColor: 'transparent',
                  '& .MuiLinearProgress-bar': {
                    background: 'linear-gradient(90deg, rgba(0,200,255,1) 0%, rgba(0,200,255,0.8) 100%)',
                    borderRadius: '4px',
                    boxShadow: '0 0 10px rgba(0,200,255,0.7)',
                    transition: 'transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)',
                  }
                }}
              />
            </Box>
          </Grid>
        </Grid>
        
        {/* Model Status Section */}
        {runner.models && runner.models.length > 0 && (
          <>
            <Divider sx={{
              my: 2,
              borderColor: 'rgba(255, 255, 255, 0.06)',
              boxShadow: '0 1px 2px rgba(0, 0, 0, 0.1)',
            }} />
            <Typography 
              variant="caption" 
              sx={{ 
                color: 'rgba(255, 255, 255, 0.6)', 
                fontWeight: 500, 
                px: 1, 
                mb: 1,
                display: 'block'
              }}
            >
              Available models:
            </Typography>
            <Grid container spacing={1} sx={{ px: 1, py: 0.5 }}>
              {runner.models.map(modelStatus => (
                <Grid item key={modelStatus.model_id}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <Tooltip title={modelStatus.error || `Runtime: ${modelStatus.runtime || 'unknown'}`} disableHoverListener={!modelStatus.error && !modelStatus.runtime}>
                      <Chip 
                        size="small"
                        label={
                          modelStatus.error 
                            ? `${modelStatus.model_id} (Error)`
                            : modelStatus.download_in_progress 
                              ? `${modelStatus.model_id} (Downloading: ${modelStatus.download_percent}%)` 
                              : `${modelStatus.model_id}${modelStatus.memory ? ` (${prettyBytes(modelStatus.memory)})` : ''}`
                        }
                        sx={{ 
                          borderRadius: '3px',
                          backgroundColor: modelStatus.error
                            ? 'rgba(255, 0, 0, 0.15)' // Red tint for error
                            : modelStatus.download_in_progress 
                              ? 'rgba(255, 165, 0, 0.15)' // Orange tint for downloading
                              : modelStatus.runtime === 'vllm'
                                ? 'rgba(147, 51, 234, 0.08)' // Purple tint for VLLM
                                : modelStatus.runtime === 'ollama'
                                  ? 'rgba(0, 200, 255, 0.08)' // Blue tint for Ollama
                                  : 'rgba(34, 197, 94, 0.08)', // Green tint for other runtimes
                          border: '1px solid',
                          borderColor: modelStatus.error
                            ? 'rgba(255, 0, 0, 0.3)' // Red border for error
                            : modelStatus.download_in_progress
                              ? 'rgba(255, 165, 0, 0.3)' // Orange border for downloading
                              : modelStatus.runtime === 'vllm'
                                ? 'rgba(147, 51, 234, 0.2)' // Purple border for VLLM
                                : modelStatus.runtime === 'ollama'
                                  ? 'rgba(0, 200, 255, 0.2)' // Blue border for Ollama
                                  : 'rgba(34, 197, 94, 0.2)', // Green border for other runtimes
                          color: modelStatus.error
                            ? 'rgba(255, 0, 0, 0.9)' // Brighter red text for error
                            : modelStatus.download_in_progress
                              ? 'rgba(255, 165, 0, 0.9)' // Brighter orange text for downloading
                              : 'rgba(255, 255, 255, 0.85)',
                          '& .MuiChip-label': {
                            fontSize: '0.7rem',
                            px: 1.2,
                          }
                        }}
                      />
                    </Tooltip>
                    {modelStatus.download_in_progress && (
                      <CircularProgress 
                        size={12} 
                        thickness={4}
                        sx={{ 
                          color: 'rgba(255, 165, 0, 0.9)'
                        }} 
                      />
                    )}
                  </Box>
                </Grid>
              ))}
            </Grid>
          </>
        )}
      </Box>
      
      {runner.slots && runner.slots.length > 0 && (
        <Box sx={{ 
          mt: 1,
          backgroundColor: 'rgba(17, 17, 19, 0.9)',
          pt: 1,
          pb: 1,
          borderTop: '1px solid rgba(255, 255, 255, 0.05)',
          boxShadow: 'inset 0 2px 4px rgba(0, 0, 0, 0.1)',
        }}>
          {runner.slots
            ?.sort((a, b) => {
              // Safely handle potentially undefined id properties
              const idA = a.id || '';
              const idB = b.id || '';
              return idA.localeCompare(idB);
            })
            .map(slot => (
              <ModelInstanceSummary
                key={slot?.id}
                slot={slot}
                models={runner.models}
                onViewSession={onViewSession}
              />
            ))
          }
        </Box>
      )}
      
      {(!runner.slots || runner.slots.length === 0) && (
        <Box sx={{ 
          backgroundColor: 'rgba(17, 17, 19, 0.9)',
          p: 3,
          textAlign: 'center',
          borderTop: '1px solid rgba(255, 255, 255, 0.05)',
          boxShadow: 'inset 0 2px 4px rgba(0, 0, 0, 0.1)',
        }}>
          <Typography 
            variant="body2" 
            sx={{
              color: 'rgba(255, 255, 255, 0.4)',
              fontStyle: 'italic',
              letterSpacing: '0.5px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 1,
              '&::before, &::after': {
                content: '""',
                height: '1px',
                width: '50px',
                background: 'linear-gradient(90deg, rgba(255,255,255,0) 0%, rgba(255,255,255,0.1) 50%, rgba(255,255,255,0) 100%)',
              }
            }}
          >
            No active model instances
          </Typography>
        </Box>
      )}
    </Paper>
  )
}

export default RunnerSummary