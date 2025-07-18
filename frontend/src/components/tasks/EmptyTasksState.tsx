import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import AlarmIcon from '@mui/icons-material/Alarm'
import AddIcon from '@mui/icons-material/Add'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import { TypesTrigger } from '../../api/api'
import { getUserTimezone } from '../../utils/cronUtils'

interface EmptyTasksStateProps {
  onCreateTask: (taskData?: {
    name: string
    schedule: string
    input: string
  }) => void
}

// Example task templates
const EXAMPLE_TASKS = [
  {
    name: 'Create a Daily Stock Tracker',
    description: 'Give me the latest updates on $TSLA, $BTC, $NVDA, $PLTR, $ETH, including current price, market cap, 24h change, and any significant news or events.',
    schedule: 'Daily at 1:10pm',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 10 13 * * 1-5`, // Weekdays at 1:10 PM
    input: 'Give me the latest updates on $TSLA, $BTC, $NVDA, $PLTR, $ETH, including current price, recent changes, and projections. Analyze sentiment, market indicators, and possible reasons for movements..'
  },
  {
    name: 'Receive a Weekly Sports Update',
    description: 'Tell me about upcoming must-watch soccer games, including location, schedule, and key players to watch.',
    schedule: 'Thursdays at 7:50pm',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 50 19 * * 4`, // Thursdays at 7:50 PM
    input: 'Tell me about upcoming must-watch soccer games, including location, schedule, and key players to watch.'
  },
  {
    name: 'Daily Weather Update',
    description: 'Provide a daily weather update for the location of the user.  ',
    schedule: 'Daily at 1:10pm',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 10 13 * * 1-5`, // Weekdays at 1:10 PM
    input: 'Provide a daily weather update for the location of the user.'
  },
  {
    name: 'Daily News Update',
    description: 'Provide a daily news update for the user.',
    schedule: 'Daily at 1:10pm',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 10 13 * * 1-5`, // Weekdays at 1:10 PM
    input: 'Provide a daily news update for the user.'
  },
  {
    name: 'Daily Productivity Tip',
    description: 'Suggest 1 actionable productivity tip for the day, tailored to someone working in a fast-paced environment. Include a motivational quote to start the day.',
    schedule: 'Daily at 8:00am',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 0 8 * * 1-5`, // Weekdays at 8:00 AM
    input: 'Suggest 1 actionable productivity tip for the day, tailored to someone working in a fast-paced environment. Include a motivational quote to start the day.'
  },
  {
    name: 'Custom Newsletter',
    description: 'Provide a concise summary of the top 5 news stories from the past 24 hours, focusing on politics, technology, and science. Include one key takeaway for each story. Format as a Custom Newsletter with date, edition, and sources from web lookups. Deliver daily.',
    schedule: 'Daily at 7:00am',
    scheduleCron: `CRON_TZ=${getUserTimezone()} 0 7 * * 1-5`, // Weekdays at 7:00 AM
    input: 'Provide a concise summary of the top 5 news stories from the past 24 hours, focusing on politics, technology, and science. Include one key takeaway for each story. Format as a Custom Newsletter with date, edition, and sources from web lookups. Deliver daily.'
  }

]

const EmptyTasksState: FC<EmptyTasksStateProps> = ({ onCreateTask }) => {
  // Get two random tasks from the example tasks
  const randomTasks = useMemo(() => {
    const shuffled = [...EXAMPLE_TASKS].sort(() => 0.5 - Math.random())
    return shuffled.slice(0, 2)
  }, [])

  const handleExampleClick = (task: typeof EXAMPLE_TASKS[0]) => {
    onCreateTask({
      name: task.name,
      schedule: task.scheduleCron,
      input: task.input
    })
  }

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
        {randomTasks.map((task, index) => (
          <Card
            key={index}
            sx={{
              width: 400,
              cursor: 'pointer',
              transition: 'all 0.2s ease-in-out',
              borderRadius: 2,
              '&:hover': {
                transform: 'translateY(-2px)',
                boxShadow: 3,
                borderColor: 'primary.main',
              },
            }}
            onClick={() => handleExampleClick(task)}
          >
            <CardContent sx={{ p: 2 }}>
              <Typography 
                variant="subtitle1" 
                sx={{ fontWeight: 600, mb: 1 }}
              >
                {task.name}
              </Typography>
              <Typography 
                variant="body2" 
                color="text.secondary"
                sx={{ mb: 1 }}
              >
                {task.description}
              </Typography>
              <Typography 
                variant="caption" 
                color="text.secondary"
              >
                {task.schedule}
              </Typography>
            </CardContent>
          </Card>
        ))}
      </Box>
    </Box>
  )
}

export default EmptyTasksState 