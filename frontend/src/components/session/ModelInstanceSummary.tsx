import React, { FC, useState, useMemo } from 'react'
import Box from '@mui/material/Box'
import { prettyBytes } from '../../utils/format'
import IconButton from '@mui/material/IconButton'
import VisibilityIcon from '@mui/icons-material/Visibility'
import Typography from '@mui/material/Typography'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import Paper from '@mui/material/Paper'
import Grid from '@mui/material/Grid'

import {
  IModelInstanceState,
  ISessionSummary,
  ISlot,
  SESSION_MODE_INFERENCE
} from '../../types'

import { TypesRunnerSlot } from '../../api/api'

import {
  getColor,
  getHeadline,
  getSessionHeadline,
  getSummaryCaption,
  getModelInstanceIdleTime,
  shortID,
} from '../../utils/session'

export const ModelInstanceSummary: FC<{
  slot: TypesRunnerSlot,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  slot,
  onViewSession,
}) => {

  const [ historyViewing, setHistoryViewing ] = useState(false)

  const statusColor = useMemo(() => {
    if (slot.active) {
      return '#F4D35E'
    }
    if (!slot.ready) {
      return '#E28000'
    }
    return '#e5e5e5'
  }, [slot.ready, slot.active])

  // Get runtime specific color for the bullet
  const runtimeColor = useMemo(() => {
    // Convert runtime to lowercase to handle any case inconsistencies
    const runtime = slot.runtime?.toLowerCase() ?? '';
    
    // Match color based on runtime
    if (runtime.includes('vllm')) {
      return '#72C99A'; // Green for VLLM
    } else if (runtime.includes('ollama')) {
      return '#F4D35E'; // Yellow for Ollama
    } else if (runtime.includes('axolotl')) {
      return '#FF6B6B'; // Red for Axolotl
    } else if (runtime.includes('diffusers')) {
      return '#D183C9'; // Purple for Diffusers
    }
    
    // Default fallback color if runtime doesn't match
    return statusColor;
  }, [slot.runtime, statusColor]);

  // Enhanced gradient border based on status
  const borderGradient = useMemo(() => {
    if (slot.active) {
      return 'linear-gradient(90deg, #F4D35E 0%, rgba(244, 211, 94, 0.7) 100%)'
    }
    if (!slot.ready) {
      return 'linear-gradient(90deg, #E28000 0%, rgba(226, 128, 0, 0.7) 100%)'
    }
    return 'linear-gradient(90deg, #e5e5e5 0%, rgba(229, 229, 229, 0.7) 100%)'
  }, [slot.ready, slot.active])

  return (
    <Paper
      elevation={0}
      sx={{
        width: 'calc(100% - 24px)',
        mx: 1.5,
        my: 1.5,
        backgroundColor: 'rgba(30, 30, 32, 0.4)',
        position: 'relative',
        overflow: 'hidden',
        borderRadius: '3px',
        border: '1px solid',
        borderColor: theme => slot.active ? statusColor : 'rgba(255, 255, 255, 0.05)',
        boxShadow: theme => slot.active ? `0 0 10px rgba(244, 211, 94, 0.15)` : 'none',
      }}
    >
      <Box sx={{ p: 2.5, pl: 3 }}>
        <Grid container spacing={1}>
          <Grid item xs>
            <Typography 
              variant="subtitle1" 
              sx={{ 
                fontWeight: 600,
                color: 'rgba(255, 255, 255, 0.9)',
                display: 'flex',
                alignItems: 'center'
              }}
            >
              <Box 
                component="span" 
                sx={{ 
                  display: 'inline-block',
                  width: 8, 
                  height: 8, 
                  borderRadius: '50%', 
                  backgroundColor: runtimeColor,
                  mr: 1.5,
                  boxShadow: theme => slot.active ? `0 0 6px ${runtimeColor}` : 'none',
                }} 
              />
              { slot.runtime }: { slot.model }
            </Typography>
          </Grid>
          <Grid item>
            <Typography 
              variant="caption" 
              sx={{ 
                color: 'rgba(255, 255, 255, 0.6)',
                fontFamily: 'monospace',
                backgroundColor: 'rgba(255, 255, 255, 0.05)',
                px: 1,
                py: 0.5,
                borderRadius: '2px'
              }}
            >
              { slot.id }
            </Typography>
          </Grid>
        </Grid>
        
        <Typography 
          variant="caption" 
          sx={{ 
            display: 'inline-block', 
            mt: 1.5,
            color: slot.active ? statusColor : 'rgba(255, 255, 255, 0.6)',
            fontWeight: slot.active ? 500 : 400,
            px: 1.5,
            py: 0.7,
            borderRadius: '3px',
            border: '1px solid',
            borderColor: slot.active ? `${statusColor}40` : 'transparent',
            backgroundColor: slot.active ? `${statusColor}10` : 'transparent'
          }}
        >
          { slot.status }
        </Typography>
      </Box>
    </Paper>
  )
}

export default ModelInstanceSummary

// TODO(phil): Old functionality
// Model Instance Display
// - Shows model name and mode via SessionBadge
// - Displays memory usage in a readable format
// - Shows current status code
// - Different border styling for active vs inactive instances
// Session Information
// - When a session is active:
//   - Displays session headline in bold
// Shows additional session summary caption
// When idle:
// Shows model headline with name, mode, and LoRA directory
// Displays idle time duration
// Job History Functionality
// Toggleable job history view with "view jobs"/"hide jobs" button
// Shows count of jobs (properly pluralized)
// History displayed in scrollable container (max height 100px)
// - Jobs are displayed in reverse chronological order
// - Per Job Entry Display
//   - Time stamp (HH:MM:SS format)
//   - Session and interaction IDs (shortened format)
//   - Interactive JSON viewer for detailed job data
//   - Quick "view session" button (eye icon) to navigate to full session view
// Layout Features
// - Responsive grid layout using Row and Cell components
// - Consistent typography styling with MUI components
// - Compact line heights (lineHeight: 1)
// - Right-aligned job history toggle
// State Management
// Uses local state for history view toggle
// Memoized job history array to prevent unnecessary recalculations