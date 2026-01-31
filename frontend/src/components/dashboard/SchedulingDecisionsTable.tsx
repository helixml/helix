import React, { FC } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Typography,
  Box,
  Tooltip,
  useTheme
} from '@mui/material'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import ErrorIcon from '@mui/icons-material/Error'
import QueueIcon from '@mui/icons-material/Queue'
import RecyclingIcon from '@mui/icons-material/Recycling'
import AddIcon from '@mui/icons-material/Add'
import CancelIcon from '@mui/icons-material/Cancel'
import ScheduleIcon from '@mui/icons-material/Schedule'
import { TypesSchedulingDecision } from '../../api/api'
// Using built-in date formatting instead of date-fns

interface SchedulingDecisionsTableProps {
  decisions: TypesSchedulingDecision[]
}

const SchedulingDecisionsTable: FC<SchedulingDecisionsTableProps> = ({ decisions }) => {
  const theme = useTheme()

  const getDecisionTypeColor = (decisionType: string, success: boolean) => {
    if (!success) return 'error'
    
    switch (decisionType) {
      case 'queued':
        return 'info'
      case 'reuse_warm_slot':
        return 'success'
      case 'create_new_slot':
        return 'warning'
      case 'rejected':
        return 'error'
      case 'error':
        return 'error'
      case 'unschedulable':
        return 'warning'
      default:
        return 'default'
    }
  }

  const getDecisionTypeIcon = (decisionType: string, success: boolean) => {
    if (!success) {
      return <ErrorIcon fontSize="small" />
    }
    
    switch (decisionType) {
      case 'queued':
        return <QueueIcon fontSize="small" />
      case 'reuse_warm_slot':
        return <RecyclingIcon fontSize="small" />
      case 'create_new_slot':
        return <AddIcon fontSize="small" />
      case 'rejected':
        return <CancelIcon fontSize="small" />
      case 'error':
        return <ErrorIcon fontSize="small" />
      case 'unschedulable':
        return <ScheduleIcon fontSize="small" />
      default:
        return <CheckCircleIcon fontSize="small" />
    }
  }

  const getDecisionTypeLabel = (decisionType: string) => {
    switch (decisionType) {
      case 'queued':
        return 'Queued'
      case 'reuse_warm_slot':
        return 'Reused Warm Slot'
      case 'create_new_slot':
        return 'Created New Slot'
      case 'rejected':
        return 'Rejected'
      case 'error':
        return 'Error'
      case 'unschedulable':
        return 'Unschedulable'
      default:
        return decisionType
    }
  }

  const formatTime = (timeString: string) => {
    try {
      const date = new Date(timeString)
      const now = new Date()
      const diffMs = now.getTime() - date.getTime()
      const diffSecs = Math.floor(diffMs / 1000)
      const diffMins = Math.floor(diffSecs / 60)
      const diffHours = Math.floor(diffMins / 60)
      
      if (diffSecs < 60) return `${diffSecs}s ago`
      if (diffMins < 60) return `${diffMins}m ago`
      if (diffHours < 24) return `${diffHours}h ago`
      return date.toLocaleString()
    } catch {
      return 'Unknown'
    }
  }

  if (!decisions || decisions.length === 0) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center', backgroundColor: 'rgba(25, 25, 28, 0.3)' }}>
        <Typography variant="body2" color="text.secondary">
          No scheduling decisions recorded yet
        </Typography>
      </Paper>
    )
  }

  return (
    <TableContainer component={Paper} sx={{ backgroundColor: 'rgba(30, 30, 32, 0.95)' }}>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Time
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Decision
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Model
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Runner
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Reason
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Processing Time
            </TableCell>
            <TableCell sx={{ color: 'rgba(255, 255, 255, 0.9)', fontWeight: 600 }}>
              Slots
            </TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {decisions.map((decision, index) => (
            <TableRow 
              key={decision.id || index}
              sx={{ 
                '&:nth-of-type(odd)': { 
                  backgroundColor: 'rgba(255, 255, 255, 0.02)' 
                },
                '&:hover': { 
                  backgroundColor: 'rgba(255, 255, 255, 0.05)' 
                }
              }}
            >
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
                <Typography variant="caption">
                  {formatTime(decision.created || '')}
                </Typography>
              </TableCell>
              <TableCell>
                <Chip
                  icon={getDecisionTypeIcon(decision.decision_type || '', decision.success || false)}
                  label={getDecisionTypeLabel(decision.decision_type || '')}
                  color={getDecisionTypeColor(decision.decision_type || '', decision.success || false)}
                  size="small"
                  sx={{ 
                    '& .MuiChip-label': { 
                      fontSize: '0.75rem' 
                    }
                  }}
                />
              </TableCell>
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.8)' }}>
                <Typography variant="body2" component="div">
                  {decision.model_name}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {decision.mode}
                </Typography>
              </TableCell>
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.8)' }}>
                {decision.runner_id ? (
                  <Tooltip title={`Runner: ${decision.runner_id}`}>
                    <Typography variant="body2">
                      {decision.runner_id.slice(0, 8)}...
                    </Typography>
                  </Tooltip>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    None
                  </Typography>
                )}
              </TableCell>
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.8)' }}>
                <Box sx={{ 
                  display: 'flex',
                  alignItems: 'center',
                  gap: 1,
                  maxWidth: 300,
                  wordBreak: 'break-word',
                  whiteSpace: 'normal'
                }}>
                  <Typography variant="body2" component="span">
                    {decision.reason}
                  </Typography>
                  {decision.repeat_count && decision.repeat_count > 0 && (
                    <Chip 
                      label={`×${decision.repeat_count + 1}`}
                      size="small"
                      color="warning"
                      sx={{ 
                        height: 20, 
                        '& .MuiChip-label': { 
                          fontSize: '0.7rem',
                          px: 0.5
                        }
                      }}
                    />
                  )}
                </Box>
              </TableCell>
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
                <Typography variant="body2">
                  {decision.processing_time_ms}ms
                </Typography>
              </TableCell>
              <TableCell sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                  <Typography variant="caption">
                    Warm: {decision.warm_slot_count || 0}
                  </Typography>
                  <Typography variant="caption">
                    Total: {decision.total_slot_count || 0}
                  </Typography>
                </Box>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

export default SchedulingDecisionsTable 