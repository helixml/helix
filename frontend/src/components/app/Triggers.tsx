import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import Grid from '@mui/material/Grid'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import ScheduleIcon from '@mui/icons-material/Schedule'
import ApiIcon from '@mui/icons-material/Api'
import { TypesTrigger } from '../../api/api'
import useThemeConfig from '../../hooks/useThemeConfig'

interface TriggersProps {
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const DAYS_OF_WEEK = [
  { value: 0, label: 'SUN', fullLabel: 'Sunday' },
  { value: 1, label: 'MON', fullLabel: 'Monday' },
  { value: 2, label: 'TUE', fullLabel: 'Tuesday' },
  { value: 3, label: 'WED', fullLabel: 'Wednesday' },
  { value: 4, label: 'THU', fullLabel: 'Thursday' },
  { value: 5, label: 'FRI', fullLabel: 'Friday' },
  { value: 6, label: 'SAT', fullLabel: 'Saturday' },
]

const HOURS = Array.from({ length: 24 }, (_, i) => i)
const MINUTES = Array.from({ length: 60 }, (_, i) => i)

// Helper functions to parse cron expressions
const parseCronDays = (cronExpression: string): number[] => {
  const parts = cronExpression.split(' ')
  if (parts.length >= 5) {
    const dayPart = parts[4] // weekday is the 5th field (index 4)
    return dayPart.split(',').map(d => parseInt(d)).filter(d => !isNaN(d))
  }
  return []
}

const parseCronHour = (cronExpression: string): number => {
  const parts = cronExpression.split(' ')
  if (parts.length >= 2) {
    return parseInt(parts[1]) || 9 // hour is the 2nd field (index 1)
  }
  return 9
}

const parseCronMinute = (cronExpression: string): number => {
  const parts = cronExpression.split(' ')
  if (parts.length >= 1) {
    return parseInt(parts[0]) || 0 // minute is the 1st field (index 0)
  }
  return 0
}

const formatCronSchedule = (schedule: string): string => {
  const days = parseCronDays(schedule)
  const hour = parseCronHour(schedule)
  const minute = parseCronMinute(schedule)
  
  if (days.length === 0) return 'Invalid schedule'
  
  const dayLabels = days.map(d => DAYS_OF_WEEK[d].fullLabel)
  return `Every ${dayLabels.join(', ')} at ${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}`
}

const Triggers: FC<TriggersProps> = ({
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasCronTrigger = triggers.some(t => t.cron)
  const cronTrigger = triggers.find(t => t.cron)?.cron

  const themeConfig = useThemeConfig()

  // State for inline schedule configuration
  const [selectedDays, setSelectedDays] = useState<number[]>(
    cronTrigger ? parseCronDays(cronTrigger.schedule || '') : [1] // Default to Monday
  )
  const [selectedHour, setSelectedHour] = useState<number>(
    cronTrigger ? parseCronHour(cronTrigger.schedule || '') : 9
  )
  const [selectedMinute, setSelectedMinute] = useState<number>(
    cronTrigger ? parseCronMinute(cronTrigger.schedule || '') : 0
  )
  const [scheduleInput, setScheduleInput] = useState<string>(
    cronTrigger?.input || ''
  )

  // Update state when triggers change
  useEffect(() => {
    if (cronTrigger) {
      setSelectedDays(parseCronDays(cronTrigger.schedule || ''))
      setSelectedHour(parseCronHour(cronTrigger.schedule || ''))
      setSelectedMinute(parseCronMinute(cronTrigger.schedule || ''))
      setScheduleInput(cronTrigger.input || '')
    }
  }, [cronTrigger])

  const handleCronToggle = (enabled: boolean) => {
    if (enabled) {
      // Add a default cron trigger (Monday at 9:00 AM)
      const newTriggers = [...triggers.filter(t => !t.cron), { cron: { schedule: '0 9 * * 1', input: '' } }]
      onUpdate(newTriggers)
    } else {
      // Remove cron trigger
      const newTriggers = triggers.filter(t => !t.cron)
      onUpdate(newTriggers)
    }
  }

  const handleDayToggle = (day: number) => {
    const newSelectedDays = selectedDays.includes(day) 
      ? selectedDays.filter(d => d !== day)
      : [...selectedDays, day].sort()
    
    setSelectedDays(newSelectedDays)
    updateCronTrigger(newSelectedDays, selectedHour, selectedMinute, scheduleInput)
  }

  const handleHourChange = (hour: number) => {
    setSelectedHour(hour)
    updateCronTrigger(selectedDays, hour, selectedMinute, scheduleInput)
  }

  const handleMinuteChange = (minute: number) => {
    setSelectedMinute(minute)
    updateCronTrigger(selectedDays, selectedHour, minute, scheduleInput)
  }

  const handleInputChange = (input: string) => {
    setScheduleInput(input)
    updateCronTrigger(selectedDays, selectedHour, selectedMinute, input)
  }

  const updateCronTrigger = (days: number[], hour: number, minute: number, input: string) => {
    if (days.length === 0) return
    
    const cronExpression = `${minute} ${hour} * * ${days.join(',')}`
    const newTriggers = [...triggers.filter(t => !t.cron), { cron: { schedule: cronExpression, input } }]
    onUpdate(newTriggers)
  }

  return (
    <Box sx={{ mt: 2, mr: 2}}>
      <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
        Triggers
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Choose how you want to interact with your agent. Direct API requests, recurring schedules, or 3rd party applications.
      </Typography>

      {/* Sessions/API Trigger - Always enabled and read-only */}
      <Box sx={{ mb: 3, p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <ApiIcon sx={{ mr: 2, color: 'primary.main' }} />
            <Box>
              <Typography gutterBottom>Sessions & API</Typography>
              <Typography variant="body2" color="text.secondary">
                Allow users to interact with your agent through the web interface or API
              </Typography>
            </Box>
          </Box>
          <FormControlLabel
            control={<Switch checked={true} disabled />}
            label=""
          />
        </Box>
      </Box>

      {/* Recurring Trigger */}
      <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <ScheduleIcon sx={{ mr: 2, color: 'primary.main' }} />
            <Box>
              <Typography gutterBottom>Recurring</Typography>
              <Typography variant="body2" color="text.secondary">
                Run your agent on a schedule
              </Typography>
            </Box>
          </Box>
          <FormControlLabel
            control={
              <Switch
                checked={hasCronTrigger}
                onChange={(e) => handleCronToggle(e.target.checked)}
                disabled={readOnly}
              />
            }
            label=""
          />
        </Box>

        {hasCronTrigger && (
          <Box sx={{ mt: 2, p: 2, borderRadius: 1 }}>
            {/* Days of the week */}
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom>
                Days of the week
              </Typography>
              <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                {DAYS_OF_WEEK.map((day) => (
                  <Button
                    key={day.value}
                    variant={selectedDays.includes(day.value) ? "contained" : "outlined"}
                    color={selectedDays.includes(day.value) ? "secondary" : "secondary"}
                    size="small"
                    onClick={() => handleDayToggle(day.value)}
                    disabled={readOnly}
                    sx={{ 
                      minWidth: 'auto', 
                      px: 1.5,
                      py: 0.5,
                      fontSize: '0.75rem'
                    }}
                  >
                    {day.label}
                  </Button>
                ))}
              </Box>
            </Box>

            {/* Time selection */}
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
                Time
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={6}>
                  <FormControl fullWidth size="small">
                    <InputLabel>Hour</InputLabel>
                    <Select
                      value={selectedHour}
                      label="Hour"
                      onChange={(e) => handleHourChange(e.target.value as number)}
                      disabled={readOnly}
                    >
                      {HOURS.map((hour) => (
                        <MenuItem key={hour} value={hour}>
                          {hour.toString().padStart(2, '0')}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Grid>
                <Grid item xs={6}>
                  <FormControl fullWidth size="small">
                    <InputLabel>Minute</InputLabel>
                    <Select
                      value={selectedMinute}
                      label="Minute"
                      onChange={(e) => handleMinuteChange(e.target.value as number)}
                      disabled={readOnly}
                    >
                      {MINUTES.map((minute) => (
                        <MenuItem key={minute} value={minute}>
                          {minute.toString().padStart(2, '0')}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Grid>
              </Grid>
            </Box>

            {/* Input field */}
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2  }}>
                Agent Input
              </Typography>
              <TextField
                fullWidth
                multiline
                rows={4}
                size="small"
                placeholder="'Check the news and send me an email with the summary', 'Check the weather in Tokyo'"
                value={scheduleInput}
                onChange={(e) => handleInputChange(e.target.value)}
                disabled={readOnly}
              />
            </Box>

            {/* Schedule summary */}
            <Box>
              <Typography variant="body2" color="text.secondary">
                <strong>Summary:</strong> {selectedDays.length > 0 
                  ? `Every ${selectedDays.map(d => DAYS_OF_WEEK[d].fullLabel).join(', ')} at ${selectedHour.toString().padStart(2, '0')}:${selectedMinute.toString().padStart(2, '0')}`
                  : 'No days selected'
                }
              </Typography>
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default Triggers
