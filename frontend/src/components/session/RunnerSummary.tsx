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
  TypesDashboardRunner,
  TypesGPUStatus
} from '../../api/api'

import ModelInstanceLogs from '../admin/ModelInstanceLogs'

const GPUCard: FC<{ gpu: TypesGPUStatus, allocatedMemory: number, allocatedModels: Array<{ model: string, memory: number, isMultiGPU: boolean }>, runner: TypesDashboardRunner }> = ({ gpu, allocatedMemory, allocatedModels, runner }) => {
  const total_memory = gpu.total_memory || 1
  const used_memory = gpu.used_memory || 0
  
  const usedPercent = Math.round((used_memory / total_memory) * 100)
  const allocatedPercent = Math.round((allocatedMemory / total_memory) * 100)
  
  // Create tooltip content for allocated models
  const allocatedTooltip = allocatedModels.length > 0 ? (
    <Box>
      <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
        Allocated Models:
      </Typography>
      {allocatedModels.map((modelInfo, idx) => (
        <Typography key={idx} variant="caption" sx={{ display: 'block', fontSize: '0.65rem' }}>
          • {modelInfo.model} ({prettyBytes(modelInfo.memory)})
          {modelInfo.isMultiGPU && <span style={{ color: '#7986cb' }}> [Multi-GPU]</span>}
        </Typography>
      ))}
    </Box>
  ) : null
  
  // Simplify GPU model name for display
  const getSimpleModelName = (fullName: string) => {
    if (!fullName || fullName === 'unknown') return 'Unknown GPU'
    
    // Extract key parts: NVIDIA H100, RTX 4090, etc.
    const name = fullName.replace(/^NVIDIA\s+/, '').replace(/\s+PCIe.*$/, '').replace(/\s+SXM.*$/, '')
    return name
  }
  
  return (
    <Box sx={{ 
      mb: 1.5,
      p: 2,
      backgroundColor: 'rgba(0, 0, 0, 0.2)',
      borderRadius: 1,
      border: '1px solid rgba(255, 255, 255, 0.05)',
      backdropFilter: 'blur(5px)',
    }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Box>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: '#00c8ff', lineHeight: 1 }}>
            GPU {gpu.index}
          </Typography>
          <Tooltip title={`${gpu.model_name || 'Unknown GPU'} • Driver: ${gpu.driver_version || 'unknown'} • CUDA: ${gpu.cuda_version || 'unknown'}`}>
            <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'rgba(255, 255, 255, 0.6)', cursor: 'help' }}>
              {getSimpleModelName(gpu.model_name || '')}
            </Typography>
          </Tooltip>
        </Box>
        <Chip 
          size="small" 
          label={`${usedPercent}%`}
          sx={{ 
            height: 18, 
            fontSize: '0.65rem',
            backgroundColor: usedPercent > 80 ? 'rgba(255, 0, 0, 0.2)' : 'rgba(0, 200, 255, 0.2)',
            color: usedPercent > 80 ? '#ff6b6b' : '#00c8ff',
            border: `1px solid ${usedPercent > 80 ? 'rgba(255, 0, 0, 0.3)' : 'rgba(0, 200, 255, 0.3)'}`,
          }}
        />
      </Box>
      
      {/* Dual progress bars - allocated (background) and used (foreground) */}
      <Box sx={{ position: 'relative', height: 12, mb: 1 }}>
        <Box sx={{ 
          position: 'absolute',
          width: '100%',
          height: '100%',
          backgroundColor: 'rgba(255, 255, 255, 0.03)',
          borderRadius: '3px',
          boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.3)',
        }} />
        
        {/* Allocated memory bar */}
        <Tooltip title={allocatedTooltip || 'No models allocated'}>
          <LinearProgress
            variant="determinate"
            value={allocatedPercent}
            sx={{ 
              width: '100%',
              height: '100%',
              borderRadius: '3px',
              backgroundColor: 'transparent',
              cursor: allocatedTooltip ? 'help' : 'default',
              '& .MuiLinearProgress-bar': {
                background: 'linear-gradient(90deg, rgba(121,134,203,0.9) 0%, rgba(121,134,203,0.7) 100%)',
                borderRadius: '3px',
                transition: 'transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)',
                boxShadow: '0 0 6px rgba(121,134,203,0.5)'
              }
            }}
          />
        </Tooltip>
        
        {/* Used memory bar */}
        <LinearProgress
          variant="determinate"
          value={usedPercent}
          sx={{ 
            position: 'absolute', 
            width: '100%', 
            height: 8,
            top: 2,
            borderRadius: '3px',
            backgroundColor: 'transparent',
            '& .MuiLinearProgress-bar': {
              background: usedPercent > 80 
                ? 'linear-gradient(90deg, rgba(255,107,107,1) 0%, rgba(255,107,107,0.8) 100%)'
                : 'linear-gradient(90deg, rgba(0,200,255,1) 0%, rgba(0,200,255,0.8) 100%)',
              borderRadius: '3px',
              boxShadow: usedPercent > 80 
                ? '0 0 8px rgba(255,107,107,0.7)'
                : '0 0 8px rgba(0,200,255,0.7)',
              transition: 'transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)',
            }
          }}
        />
      </Box>
      
      {/* Memory details */}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <Typography variant="caption" sx={{ fontSize: '0.7rem', color: '#00c8ff', fontWeight: 600 }}>
            Used: {prettyBytes(used_memory)} ({usedPercent}%)
          </Typography>
        </Box>
        {allocatedMemory > 0 && (
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="caption" sx={{ fontSize: '0.7rem', color: '#7986cb', fontWeight: 600 }}>
              Allocated: {prettyBytes(allocatedMemory)} ({allocatedPercent}%)
            </Typography>
          </Box>
        )}
        <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <Typography variant="caption" sx={{ fontSize: '0.7rem', color: 'rgba(255, 255, 255, 0.4)' }}>
            Total: {prettyBytes(total_memory)}
          </Typography>
        </Box>
      </Box>
    </Box>
  )
}

// Calculate allocated memory per GPU based on running slots
const calculateGPUAllocatedMemory = (runner: TypesDashboardRunner): Map<number, number> => {
  const gpuAllocatedMemory = new Map<number, number>()
  
  if (!runner.slots || !runner.models) {
    return gpuAllocatedMemory
  }
  
  // Create a map of model ID to memory requirement
  const modelMemoryMap = new Map<string, number>()
  runner.models.forEach(model => {
    if (model.model_id && model.memory) {
      modelMemoryMap.set(model.model_id, model.memory)
    }
  })
  
  // Calculate allocated memory per GPU
  runner.slots.forEach(slot => {
    if (!slot.model) return
    
    const modelMemory = modelMemoryMap.get(slot.model)
    if (!modelMemory) return
    
    if (slot.gpu_indices && slot.gpu_indices.length > 1) {
      // Multi-GPU model: distribute memory across GPUs
      const memoryPerGPU = modelMemory / slot.gpu_indices.length
      slot.gpu_indices.forEach(gpuIndex => {
        const current = gpuAllocatedMemory.get(gpuIndex) || 0
        gpuAllocatedMemory.set(gpuIndex, current + memoryPerGPU)
      })
    } else if (slot.gpu_index !== undefined) {
      // Single GPU model: allocate full memory to this GPU
      const current = gpuAllocatedMemory.get(slot.gpu_index) || 0
      gpuAllocatedMemory.set(slot.gpu_index, current + modelMemory)
    }
  })
  
  return gpuAllocatedMemory
}

// Calculate which models are allocated to each GPU
const calculateGPUAllocatedModels = (runner: TypesDashboardRunner): Map<number, Array<{ model: string, memory: number, isMultiGPU: boolean }>> => {
  const gpuAllocatedModels = new Map<number, Array<{ model: string, memory: number, isMultiGPU: boolean }>>()
  
  if (!runner.slots || !runner.models) {
    return gpuAllocatedModels
  }
  
  // Create a map of model ID to memory requirement
  const modelMemoryMap = new Map<string, number>()
  runner.models.forEach(model => {
    if (model.model_id && model.memory) {
      modelMemoryMap.set(model.model_id, model.memory)
    }
  })
  
  // Track models per GPU
  runner.slots.forEach(slot => {
    if (!slot.model) return
    
    const modelMemory = modelMemoryMap.get(slot.model)
    if (!modelMemory) return
    
    if (slot.gpu_indices && slot.gpu_indices.length > 1) {
      // Multi-GPU model: add to all GPUs
      const memoryPerGPU = modelMemory / slot.gpu_indices.length
      slot.gpu_indices.forEach(gpuIndex => {
        if (!gpuAllocatedModels.has(gpuIndex)) {
          gpuAllocatedModels.set(gpuIndex, [])
        }
        gpuAllocatedModels.get(gpuIndex)!.push({
          model: slot.model!,
          memory: memoryPerGPU,
          isMultiGPU: true
        })
      })
    } else if (slot.gpu_index !== undefined) {
      // Single GPU model: add to specific GPU
      if (!gpuAllocatedModels.has(slot.gpu_index)) {
        gpuAllocatedModels.set(slot.gpu_index, [])
      }
      gpuAllocatedModels.get(slot.gpu_index)!.push({
        model: slot.model,
        memory: modelMemory,
        isMultiGPU: false
      })
    }
  })
  
  return gpuAllocatedModels
}

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
  
  // DEBUG: Log the entire runner object
  console.log('DEBUG: Full runner object:', runner)
    
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
        
        {/* Process Cleanup Stats Section */}
        {console.log('DEBUG: runner.process_stats =', runner.process_stats) || runner.process_stats && (
          <>
            <Box sx={{ mb: 3 }}>
              <Typography 
                variant="subtitle2" 
                sx={{ 
                  mb: 2,
                  color: 'rgba(255, 255, 255, 0.8)',
                  fontWeight: 600,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 1,
                }}
              >
                Process Management
                <Chip 
                  size="small" 
                  label={`${runner.process_stats.total_tracked_processes || 0} tracked`}
                  sx={{ 
                    height: 20, 
                    fontSize: '0.65rem',
                    backgroundColor: 'rgba(0, 200, 255, 0.2)',
                    color: '#00c8ff',
                    border: '1px solid rgba(0, 200, 255, 0.3)',
                  }}
                />
              </Typography>
              
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <Box sx={{ 
                    p: 2, 
                    backgroundColor: 'rgba(0, 0, 0, 0.2)', 
                    borderRadius: '6px',
                    border: '1px solid rgba(255, 255, 255, 0.05)',
                  }}>
                    <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.7)', mb: 1 }}>
                      Cleanup Statistics
                    </Typography>
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mb: 2 }}>
                      <Chip 
                        size="small"
                        label={`${runner.process_stats.cleanup_stats?.total_cleaned || 0} processes cleaned`}
                        sx={{ 
                          height: 24, 
                          fontSize: '0.7rem',
                          backgroundColor: (runner.process_stats.cleanup_stats?.total_cleaned || 0) > 0 
                            ? 'rgba(255, 152, 0, 0.15)' 
                            : 'rgba(76, 175, 80, 0.15)',
                          color: (runner.process_stats.cleanup_stats?.total_cleaned || 0) > 0 
                            ? '#FF9800' 
                            : '#4CAF50',
                          border: `1px solid ${(runner.process_stats.cleanup_stats?.total_cleaned || 0) > 0 
                            ? 'rgba(255, 152, 0, 0.3)' 
                            : 'rgba(76, 175, 80, 0.3)'}`,
                        }}
                      />
                      <Chip 
                        size="small"
                        label={`${runner.process_stats.cleanup_stats?.synchronous_runs || 0} sync runs`}
                        sx={{ 
                          height: 24, 
                          fontSize: '0.7rem',
                          backgroundColor: 'rgba(156, 39, 176, 0.15)',
                          color: '#9C27B0',
                          border: '1px solid rgba(156, 39, 176, 0.3)',
                        }}
                      />
                      <Chip 
                        size="small"
                        label={`${runner.process_stats.cleanup_stats?.asynchronous_runs || 0} async runs`}
                        sx={{ 
                          height: 24, 
                          fontSize: '0.7rem',
                          backgroundColor: 'rgba(63, 81, 181, 0.15)',
                          color: '#3F51B5',
                          border: '1px solid rgba(63, 81, 181, 0.3)',
                        }}
                      />
                    </Box>
                    {runner.process_stats.cleanup_stats?.last_cleanup_time && (
                      <Typography variant="caption" sx={{ 
                        color: 'rgba(255, 255, 255, 0.5)', 
                        fontSize: '0.7rem',
                        display: 'block'
                      }}>
                        Last cleanup: {new Date(runner.process_stats.cleanup_stats.last_cleanup_time).toLocaleString()}
                      </Typography>
                    )}
                  </Box>
                </Grid>
                
                {runner.process_stats.cleanup_stats?.recent_cleanups && 
                 runner.process_stats.cleanup_stats.recent_cleanups.length > 0 && (
                  <Grid item xs={12} md={6}>
                    <Box sx={{ 
                      p: 2, 
                      backgroundColor: 'rgba(0, 0, 0, 0.2)', 
                      borderRadius: '6px',
                      border: '1px solid rgba(255, 255, 255, 0.05)',
                    }}>
                      <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.7)', mb: 1 }}>
                        Recent Cleanups ({runner.process_stats.cleanup_stats.recent_cleanups.length})
                      </Typography>
                      <Box sx={{ maxHeight: '120px', overflowY: 'auto' }}>
                        {runner.process_stats.cleanup_stats.recent_cleanups.slice(-5).reverse().map((cleanup: any, index: number) => (
                          <Box key={index} sx={{ 
                            mb: 1, 
                            p: 1, 
                            backgroundColor: 'rgba(255, 255, 255, 0.03)',
                            borderRadius: '4px',
                            border: '1px solid rgba(255, 255, 255, 0.05)',
                          }}>
                            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.5 }}>
                              <Typography variant="caption" sx={{ 
                                color: '#00c8ff', 
                                fontWeight: 600,
                                fontSize: '0.7rem'
                              }}>
                                PID {cleanup.pid}
                              </Typography>
                              <Chip 
                                size="small"
                                label={cleanup.method}
                                sx={{ 
                                  height: 16, 
                                  fontSize: '0.6rem',
                                  backgroundColor: cleanup.method === 'graceful' 
                                    ? 'rgba(76, 175, 80, 0.2)' 
                                    : 'rgba(244, 67, 54, 0.2)',
                                  color: cleanup.method === 'graceful' ? '#4CAF50' : '#F44336',
                                  border: `1px solid ${cleanup.method === 'graceful' 
                                    ? 'rgba(76, 175, 80, 0.3)' 
                                    : 'rgba(244, 67, 54, 0.3)'}`,
                                }}
                              />
                            </Box>
                            <Typography variant="caption" sx={{ 
                              color: 'rgba(255, 255, 255, 0.4)', 
                              fontSize: '0.65rem',
                              display: 'block',
                              mb: 0.5,
                              wordBreak: 'break-all'
                            }}>
                              {cleanup.command.length > 60 ? `${cleanup.command.substring(0, 60)}...` : cleanup.command}
                            </Typography>
                            <Typography variant="caption" sx={{ 
                              color: 'rgba(255, 255, 255, 0.3)', 
                              fontSize: '0.6rem'
                            }}>
                              {new Date(cleanup.cleaned_at).toLocaleString()}
                            </Typography>
                          </Box>
                        ))}
                      </Box>
                    </Box>
                  </Grid>
                )}
              </Grid>
            </Box>
            
            <Divider sx={{ 
              my: 2, 
              borderColor: 'rgba(255, 255, 255, 0.06)',
              boxShadow: '0 1px 2px rgba(0, 0, 0, 0.1)', 
            }} />
          </>
        )}
        
        {/* GPU Memory Section */}
        {runner.gpus && runner.gpus.length > 0 ? (
          <Box>
            <Typography 
              variant="subtitle2" 
              sx={{ 
                mb: 2,
                color: 'rgba(255, 255, 255, 0.8)',
                fontWeight: 600,
                display: 'flex',
                alignItems: 'center',
                gap: 1,
              }}
            >
              GPU Memory Usage
              <Chip 
                size="small" 
                label={`${runner.gpus.length} GPUs`}
                sx={{ 
                  height: 20, 
                  fontSize: '0.65rem',
                  backgroundColor: 'rgba(0, 200, 255, 0.2)',
                  color: '#00c8ff',
                  border: '1px solid rgba(0, 200, 255, 0.3)',
                }}
              />
            </Typography>
            
            <Grid container spacing={2}>
              {(() => {
                const gpuAllocatedMemory = calculateGPUAllocatedMemory(runner)
                const gpuAllocatedModels = calculateGPUAllocatedModels(runner)
                // Sort GPUs by index to ensure consistent ordering (GPU 0, GPU 1, etc.)
                const sortedGPUs = [...(runner.gpus || [])].sort((a, b) => (a.index || 0) - (b.index || 0))
                return sortedGPUs.map((gpu) => (
                  <Grid item xs={12} sm={6} md={4} key={gpu.index}>
                    <GPUCard 
                      gpu={gpu} 
                      allocatedMemory={gpuAllocatedMemory.get(gpu.index || 0) || 0}
                      allocatedModels={gpuAllocatedModels.get(gpu.index || 0) || []}
                      runner={runner}
                    />
                  </Grid>
                ))
              })()}
            </Grid>
          </Box>
        ) : (
          // Fallback to aggregated memory display
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
        )}
        
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
      
      {/* Model Instance Logs */}
      <Box sx={{ mt: 3 }}>
        <ModelInstanceLogs runner={runner} />
      </Box>

    </Paper>
  )
}

export default RunnerSummary