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

import {
  IModelInstanceState,
  ISessionSummary,
  ISlot
} from '../../types'

import {
  getColor,
  getHeadline,
  getSessionHeadline,
  getSummaryCaption,
  getModelInstanceIdleTime,
  shortID,
} from '../../utils/session'

export const ModelInstanceSummary: FC<{
  slot: ISlot,
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

  return (
    <Box
      sx={{
        width: '100%',
        p: 1,
        border: `1px solid ${statusColor}`,
        mt: 1,
        mb: 1,
      }}
    >
      <Row>
      <Cell>
          <Typography variant="h6" sx={{mr: 2}}>{ slot.runtime }: { slot.model }</Typography>
        </Cell>
        <Cell flexGrow={1} />
        <Cell>
          <Typography variant="caption" gutterBottom>{ slot.id }</Typography>
        </Cell>
      </Row>
      <Row>
        <Cell>
          <Typography variant="caption" gutterBottom>{ slot.status }</Typography>
        </Cell>
      </Row>
    </Box>
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