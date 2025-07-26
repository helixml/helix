import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Collapse from '@mui/material/Collapse'
import LinearProgress from '@mui/material/LinearProgress'
import Chip from '@mui/material/Chip'
import Tooltip from '@mui/material/Tooltip'
import CircularProgress from '@mui/material/CircularProgress'
import MinimizeIcon from '@mui/icons-material/Minimize'
import MaximizeIcon from '@mui/icons-material/CropFree'
import CloseIcon from '@mui/icons-material/Close'
import DnsIcon from '@mui/icons-material/Dns'
import LaunchIcon from '@mui/icons-material/Launch'
import { prettyBytes } from '../../utils/format'
import { useGetDashboardData } from '../../services/dashboardService'
import { TypesDashboardRunner } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import { useFloatingRunnerState } from '../../contexts/floatingRunnerState'

interface FloatingRunnerStateProps {
  onClose?: () => void
}

const FloatingRunnerState: FC<FloatingRunnerStateProps> = ({ onClose }) => {
  const floatingRunnerState = useFloatingRunnerState()
  const [isMinimized, setIsMinimized] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  
  // Use click position from context if available, otherwise use default position
  const getInitialPosition = () => {
    if (floatingRunnerState.clickPosition) {
      return {
        x: Math.max(0, Math.min(floatingRunnerState.clickPosition.x, window.innerWidth - 340)),
        y: Math.max(0, Math.min(floatingRunnerState.clickPosition.y, window.innerHeight - 420))
      }
    }
    return { 
      x: window.innerWidth - 340,
      y: window.innerHeight - 420
    }
  }
  
  const [position, setPosition] = useState(getInitialPosition)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  
  // Update position when click position changes
  useEffect(() => {
    if (floatingRunnerState.clickPosition) {
      setPosition({
        x: Math.max(0, Math.min(floatingRunnerState.clickPosition.x, window.innerWidth - 340)),
        y: Math.max(0, Math.min(floatingRunnerState.clickPosition.y, window.innerHeight - 420))
      })
    }
  }, [floatingRunnerState.clickPosition])
  
  const { data: dashboardData, isLoading } = useGetDashboardData()
  const router = useRouter()
  
  const handleMouseDown = (e: React.MouseEvent) => {
    setIsDragging(true)
    setDragOffset({
      x: e.clientX - position.x,
      y: e.clientY - position.y
    })
  }

  const handleViewFullDashboard = () => {
    router.navigate('dashboard', { tab: 'runners' })
  }

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDragging) {
        setPosition({
          x: e.clientX - dragOffset.x,
          y: e.clientY - dragOffset.y
        })
      }
    }

    const handleMouseUp = () => {
      setIsDragging(false)
    }

    if (isDragging) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, dragOffset])

  const CompactRunnerDisplay: FC<{ runner: TypesDashboardRunner }> = ({ runner }) => {
    const total_memory = runner.total_memory || 1
    const used_memory = typeof runner.used_memory === 'number' ? runner.used_memory : 0
    const allocated_memory = (!runner.slots || runner.slots.length === 0 || !runner.allocated_memory) 
      ? 0 
      : runner.allocated_memory
    
    const actualPercent = Math.round((used_memory / total_memory) * 100)
    const allocatedPercent = Math.round((allocated_memory / total_memory) * 100)
    
    const getModelMemory = (slotModel: string): number | undefined => {
      if (!runner.models) return undefined
      
      const matchingModel = runner.models.find(model => 
        model.model_id === slotModel || 
        model.model_id?.split(':')[0] === slotModel?.split(':')[0]
      )
      
      return matchingModel?.memory
    }
    
    return (
      <Box sx={{ mb: 1, p: 1, backgroundColor: 'rgba(0, 0, 0, 0.1)', borderRadius: 1 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 0.5 }}>
          <DnsIcon sx={{ fontSize: 14, mr: 0.5, color: '#00c8ff' }} />
          <Typography variant="caption" sx={{ fontWeight: 600, mr: 1, fontSize: '0.7rem' }}>
            {runner.id?.slice(0, 8)}...
          </Typography>
          <Chip 
            size="small" 
            label={`${actualPercent}%`}
            sx={{ 
              height: 16, 
              fontSize: '0.6rem',
              backgroundColor: actualPercent > 80 ? 'rgba(255, 0, 0, 0.2)' : 'rgba(0, 200, 255, 0.2)',
              color: actualPercent > 80 ? '#ff6b6b' : '#00c8ff',
              border: `1px solid ${actualPercent > 80 ? 'rgba(255, 0, 0, 0.3)' : 'rgba(0, 200, 255, 0.3)'}`,
            }}
          />
        </Box>
        
        <Box sx={{ position: 'relative', height: 12, mb: 0.5 }}>
          <Box sx={{ 
            position: 'absolute',
            width: '100%',
            height: '100%',
            backgroundColor: 'rgba(255, 255, 255, 0.03)',
            borderRadius: '4px',
            boxShadow: 'inset 0 1px 3px rgba(0,0,0,0.3)',
          }} />
          
          <LinearProgress
            variant="determinate"
            value={allocatedPercent}
            sx={{ 
              width: '100%',
              height: '100%',
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
          
          <LinearProgress
            variant="determinate"
            value={actualPercent}
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
        
        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
          <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'rgba(255, 255, 255, 0.6)' }}>
            Used: {prettyBytes(used_memory)} ({actualPercent}%)
          </Typography>
          <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'rgba(255, 255, 255, 0.4)' }}>
            Total: {prettyBytes(total_memory)}
          </Typography>
        </Box>
        
        {allocated_memory > 0 && (
          <Typography variant="caption" sx={{ fontSize: '0.6rem', color: '#7986cb', display: 'block', mb: 0.5 }}>
            Allocated: {prettyBytes(allocated_memory)} ({allocatedPercent}%)
          </Typography>
        )}
        
        {runner.slots && runner.slots.length > 0 && (
          <Box sx={{ mt: 0.5 }}>
            <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'rgba(255, 255, 255, 0.4)', mb: 0.25, display: 'block' }}>
              Running Models:
            </Typography>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.25 }}>
              {runner.slots.slice(0, 3).map((slot, index) => {
                const modelMemory = getModelMemory(slot.model || '')
                const modelName = slot.model?.split(':')[0] || slot.runtime || 'unknown'
                const memoryDisplay = modelMemory ? ` (${prettyBytes(modelMemory)})` : ''
                const isLoading = !slot.ready && !slot.active
                
                return (
                  <Box key={slot.id || index} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                    <Chip 
                      size="small"
                      label={`${modelName}${memoryDisplay}`}
                      sx={{ 
                        height: 14, 
                        fontSize: '0.55rem',
                        backgroundColor: slot.active 
                          ? 'rgba(244, 211, 94, 0.15)'
                          : slot.ready 
                            ? 'rgba(0, 200, 255, 0.15)'
                            : 'rgba(226, 128, 0, 0.15)',
                        color: slot.active 
                          ? '#F4D35E' 
                          : slot.ready 
                            ? '#00c8ff' 
                            : '#E28000',
                        border: `1px solid ${
                          slot.active 
                            ? 'rgba(244, 211, 94, 0.3)' 
                          : slot.ready 
                            ? 'rgba(0, 200, 255, 0.3)' 
                            : 'rgba(226, 128, 0, 0.3)'
                        }`,
                        '& .MuiChip-label': {
                          px: 0.5,
                        }
                      }}
                    />
                    {isLoading && (
                      <CircularProgress 
                        size={8} 
                        thickness={6}
                        sx={{ 
                          color: '#E28000',
                          ml: 0.25
                        }} 
                      />
                    )}
                  </Box>
                )
              })}
              {runner.slots.length > 3 && (
                <Typography variant="caption" sx={{ fontSize: '0.55rem', color: 'rgba(255, 255, 255, 0.4)' }}>
                  +{runner.slots.length - 3}
                </Typography>
              )}
            </Box>
          </Box>
        )}
        
        {(!runner.slots || runner.slots.length === 0) && (
          <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'rgba(255, 255, 255, 0.3)', fontStyle: 'italic' }}>
            No active model instances
          </Typography>
        )}
      </Box>
    )
  }

  return (
    <Paper
      elevation={8}
      sx={{
        position: 'fixed',
        left: position.x,
        top: position.y,
        width: isMinimized ? 200 : 320,
        maxHeight: isMinimized ? 50 : 400,
        backgroundColor: 'rgba(20, 20, 23, 0.95)',
        backdropFilter: 'blur(12px)',
        borderRadius: 2,
        border: '1px solid rgba(0, 200, 255, 0.3)',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.05)',
        zIndex: 9999,
        cursor: isDragging ? 'grabbing' : 'default',
        transition: 'width 0.3s ease, max-height 0.3s ease',
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          p: 1,
          backgroundColor: 'rgba(0, 200, 255, 0.1)',
          borderBottom: '1px solid rgba(0, 200, 255, 0.2)',
          cursor: 'grab',
          userSelect: 'none',
        }}
        onMouseDown={handleMouseDown}
      >
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <DnsIcon sx={{ fontSize: 16, mr: 0.5, color: '#00c8ff' }} />
          <Typography variant="caption" sx={{ fontWeight: 600, color: '#fff', fontSize: '0.75rem' }}>
            Runner State
          </Typography>
          {dashboardData?.runners && (
            <Chip 
              size="small" 
              label={dashboardData.runners.length}
              sx={{ 
                ml: 1, 
                height: 18, 
                fontSize: '0.6rem',
                backgroundColor: 'rgba(0, 200, 255, 0.2)',
                color: '#00c8ff',
                border: '1px solid rgba(0, 200, 255, 0.3)',
              }}
            />
          )}
          {dashboardData?.queue && dashboardData.queue.length > 0 && (
            <Chip 
              size="small" 
              label={`Q: ${dashboardData.queue.length}`}
              sx={{ 
                ml: 0.5, 
                height: 18, 
                fontSize: '0.6rem',
                backgroundColor: 'rgba(255, 165, 0, 0.2)',
                color: '#FFA500',
                border: '1px solid rgba(255, 165, 0, 0.3)',
              }}
            />
          )}
        </Box>
        
        <Box>
          <Tooltip title="Open full dashboard">
            <IconButton 
              size="small" 
              onClick={handleViewFullDashboard}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.25 }}
            >
              <LaunchIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={isMinimized ? "Maximize" : "Minimize"}>
            <IconButton 
              size="small" 
              onClick={() => setIsMinimized(!isMinimized)}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.25 }}
            >
              {isMinimized ? <MaximizeIcon fontSize="small" /> : <MinimizeIcon fontSize="small" />}
            </IconButton>
          </Tooltip>
          {onClose && (
            <Tooltip title="Close">
              <IconButton 
                size="small" 
                onClick={onClose}
                sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.25, ml: 0.5 }}
              >
                <CloseIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </Box>

      <Collapse in={!isMinimized}>
        <Box sx={{ p: 1.5, maxHeight: 350, overflowY: 'auto' }}>
          {isLoading ? (
            <Box sx={{ textAlign: 'center', py: 2 }}>
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                Loading runners...
              </Typography>
            </Box>
          ) : !dashboardData?.runners || dashboardData.runners.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 2 }}>
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                No active runners
              </Typography>
            </Box>
          ) : (
            <>
              <Typography variant="caption" sx={{ 
                color: 'rgba(255, 255, 255, 0.7)', 
                mb: 1, 
                display: 'block',
                fontSize: '0.7rem'
              }}>
                {dashboardData.runners.length} Active Runner{dashboardData.runners.length !== 1 ? 's' : ''}
              </Typography>
              
              {dashboardData.runners.map((runner: TypesDashboardRunner) => (
                <CompactRunnerDisplay key={runner.id} runner={runner} />
              ))}
              
              {dashboardData.queue && dashboardData.queue.length > 0 && (
                <>
                  <Box sx={{ mt: 2, mb: 1 }}>
                    <Typography variant="caption" sx={{ 
                      color: 'rgba(255, 255, 255, 0.7)', 
                      fontSize: '0.7rem',
                      display: 'block'
                    }}>
                      Queue ({dashboardData.queue.length} pending)
                    </Typography>
                    <Box sx={{ mt: 0.5, display: 'flex', flexWrap: 'wrap', gap: 0.25 }}>
                      {dashboardData.queue.slice(0, 3).map((workload, index) => (
                        <Chip 
                          key={workload.id || index}
                          size="small"
                          label={workload.model_name || 'Unknown'}
                          sx={{ 
                            height: 14, 
                            fontSize: '0.55rem',
                            backgroundColor: 'rgba(255, 165, 0, 0.15)',
                            color: '#FFA500',
                            border: '1px solid rgba(255, 165, 0, 0.3)',
                            '& .MuiChip-label': {
                              px: 0.5,
                            }
                          }}
                        />
                      ))}
                      {dashboardData.queue.length > 3 && (
                        <Typography variant="caption" sx={{ fontSize: '0.55rem', color: 'rgba(255, 255, 255, 0.4)' }}>
                          +{dashboardData.queue.length - 3}
                        </Typography>
                      )}
                    </Box>
                  </Box>
                </>
              )}
            </>
          )}
        </Box>
      </Collapse>
    </Paper>
  )
}

export default FloatingRunnerState 