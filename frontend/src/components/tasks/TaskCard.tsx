import React, { FC } from 'react'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import ScheduleIcon from '@mui/icons-material/Schedule'
import { TypesTriggerConfiguration } from '../../api/api'

interface TaskCardProps {
  task: TypesTriggerConfiguration
  onClick: (task: TypesTriggerConfiguration) => void
}

const TaskCard: FC<TaskCardProps> = ({ task, onClick }) => {
  const handleClick = () => {
    onClick(task)
  }

  // Extract schedule from cron trigger
  const getScheduleText = () => {
    if (task.trigger?.cron?.schedule) {
      // For now, just show the raw schedule
      // TODO: Parse and format the cron schedule to be more readable
      return task.trigger.cron.schedule
    }
    return 'No schedule set'
  }

  // Get next run time (placeholder for now)
  const getNextRun = () => {
    // TODO: Calculate next run time from cron schedule
    return 'Next run: TBD'
  }

  return (
    <Card 
      sx={{ 
        cursor: 'pointer',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: '0 4px 20px rgba(0,0,0,0.3)',
        },
        bgcolor: 'background.paper',
        border: '1px solid',
        borderColor: 'divider',
      }}
      onClick={handleClick}
    >
      <CardContent sx={{ p: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <ScheduleIcon 
            sx={{ 
              color: 'success.main', 
              fontSize: 20, 
              mr: 1 
            }} 
          />
          <Typography 
            variant="h6" 
            component="div"
            sx={{ 
              fontWeight: 600,
              color: 'text.primary',
              flex: 1
            }}
          >
            {task.name || 'Unnamed Task'}
          </Typography>
        </Box>
        
        {task.description && (
          <Typography 
            variant="body2" 
            color="text.secondary"
            sx={{ mb: 1 }}
          >
            {task.description}
          </Typography>
        )}
        
        <Typography 
          variant="caption" 
          color="text.secondary"
          sx={{ display: 'block' }}
        >
          {getScheduleText()}
        </Typography>
        
        <Typography 
          variant="caption" 
          color="text.secondary"
          sx={{ display: 'block', mt: 0.5 }}
        >
          {getNextRun()}
        </Typography>
      </CardContent>
    </Card>
  )
}

export default TaskCard 