import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import AlarmIcon from '@mui/icons-material/Alarm'
import AddIcon from '@mui/icons-material/Add'

interface EmptyTasksStateProps {
  onCreateTask: () => void
}

const EmptyTasksState: FC<EmptyTasksStateProps> = ({ onCreateTask }) => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        py: 8,
        px: 4,
        textAlign: 'center',
      }}
    >
      <Box
        sx={{
          width: 60,
          height: 60,
          borderRadius: '50%',
          bgcolor: 'action.hover',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          mb: 3,
        }}
      >
        <AlarmIcon 
          sx={{ 
            fontSize: 32, 
            color: 'text.secondary',
            opacity: 0.6
          }} 
        />
      </Box>
      
      <Typography 
        variant="h5" 
        component="h2"
        sx={{ 
          fontWeight: 600,
          mb: 2,
          color: 'text.primary'
        }}
      >
        Get started by adding a task
      </Typography>
      
      <Typography 
        variant="body1" 
        color="text.secondary"
        sx={{ 
          maxWidth: 400,
          mb: 4,
          lineHeight: 1.6
        }}
      >
        Schedule a task to automate actions and get reminders when they complete.
      </Typography>
      
      <Box
        sx={{
          display: 'flex',
          gap: 2,
          flexWrap: 'wrap',
          justifyContent: 'center',
        }}
      >
        {/* Example task cards */}
        <Box
          sx={{
            width: 280,
            p: 2,
            borderRadius: 2,
            bgcolor: 'background.paper',
            border: '1px solid',
            borderColor: 'divider',
            textAlign: 'left',
          }}
        >
          <Typography 
            variant="subtitle1" 
            sx={{ fontWeight: 600, mb: 1 }}
          >
            Create a Daily Stock Tracker
          </Typography>
          <Typography 
            variant="body2" 
            color="text.secondary"
            sx={{ mb: 1 }}
          >
            Give me the latest updates on $TSLA, $BTC, $NVDA, $PLTR, $ETH, including current price,...
          </Typography>
          <Typography 
            variant="caption" 
            color="text.secondary"
          >
            Daily at 1:10pm
          </Typography>
        </Box>
        
        <Box
          sx={{
            width: 280,
            p: 2,
            borderRadius: 2,
            bgcolor: 'background.paper',
            border: '1px solid',
            borderColor: 'divider',
            textAlign: 'left',
          }}
        >
          <Typography 
            variant="subtitle1" 
            sx={{ fontWeight: 600, mb: 1 }}
          >
            Receive a Weekly Sports Update
          </Typography>
          <Typography 
            variant="body2" 
            color="text.secondary"
            sx={{ mb: 1 }}
          >
            Tell me about upcoming must-watch soccer games, including location, schedule, and key...
          </Typography>
          <Typography 
            variant="caption" 
            color="text.secondary"
          >
            Thursdays at 7:50pm
          </Typography>
        </Box>
      </Box>
    </Box>
  )
}

export default EmptyTasksState 