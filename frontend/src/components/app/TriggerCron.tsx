import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import ScheduleIcon from '@mui/icons-material/Schedule'
import Alert from '@mui/material/Alert'
import { TypesTrigger } from '../../api/api'
import { timezones } from '../../utils/timezones'
import { 
  parseCronDays, 
  parseCronHour, 
  parseCronMinute, 
  parseCronTimezone, 
  parseCronInterval, 
  isIntervalCron, 
  getUserTimezone,
  generateCronSummary 
} from '../../utils/cronUtils'

interface TriggerCronProps {
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
  showTitle?: boolean
  showToggle?: boolean
  defaultEnabled?: boolean
  showBorder?: boolean
  borderRadius?: number
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
const INTERVAL_MINUTES = [5, 10, 15, 30, 60, 120, 240] // 5min to 4h

type ScheduleMode = 'intervals' | 'specific_time'

const TriggerCron: FC<TriggerCronProps> = ({
  triggers = [],
  onUpdate,
  readOnly = false,
  showTitle = true,
  showToggle = true,
  defaultEnabled = false,
  showBorder = true,
  borderRadius = 1
}) => {
  const hasCronTrigger = triggers.some(t => t.cron && t.cron.enabled === true)
  const cronTrigger = triggers.find(t => t.cron)?.cron

  // Determine initial schedule mode
  const getInitialScheduleMode = (): ScheduleMode => {
    if (!cronTrigger?.schedule) return 'specific_time'
    return isIntervalCron(cronTrigger.schedule) ? 'intervals' : 'specific_time'
  }

  // State for inline schedule configuration
  const [scheduleMode, setScheduleMode] = useState<ScheduleMode>(getInitialScheduleMode())
  const [selectedDays, setSelectedDays] = useState<number[]>(
    cronTrigger && !isIntervalCron(cronTrigger.schedule || '') ? parseCronDays(cronTrigger.schedule || '') : [1] // Default to Monday
  )
  const [selectedHour, setSelectedHour] = useState<number>(
    cronTrigger && !isIntervalCron(cronTrigger.schedule || '') ? parseCronHour(cronTrigger.schedule || '') : 9
  )
  const [selectedMinute, setSelectedMinute] = useState<number>(
    cronTrigger && !isIntervalCron(cronTrigger.schedule || '') ? parseCronMinute(cronTrigger.schedule || '') : 0
  )
  const [selectedTimezone, setSelectedTimezone] = useState<string>(
    cronTrigger && !isIntervalCron(cronTrigger.schedule || '') ? parseCronTimezone(cronTrigger.schedule || '') : getUserTimezone()
  )
  const [selectedInterval, setSelectedInterval] = useState<number>(
    cronTrigger && isIntervalCron(cronTrigger.schedule || '') ? parseCronInterval(cronTrigger.schedule || '') : 5
  )
  const [scheduleInput, setScheduleInput] = useState<string>(
    cronTrigger?.input || ''
  )

  // Update state when triggers change
  useEffect(() => {
    if (cronTrigger) {
      const isInterval = isIntervalCron(cronTrigger.schedule || '')
      setScheduleMode(isInterval ? 'intervals' : 'specific_time')
      
      if (isInterval) {
        setSelectedInterval(parseCronInterval(cronTrigger.schedule || ''))
      } else {
        setSelectedDays(parseCronDays(cronTrigger.schedule || ''))
        setSelectedHour(parseCronHour(cronTrigger.schedule || ''))
        setSelectedMinute(parseCronMinute(cronTrigger.schedule || ''))
        setSelectedTimezone(parseCronTimezone(cronTrigger.schedule || ''))
      }
      setScheduleInput(cronTrigger.input || '')
    }
  }, [cronTrigger])

  // Handle defaultEnabled prop
  useEffect(() => {
    if (defaultEnabled && !hasCronTrigger && triggers.length === 0) {
      // Create a default cron trigger when defaultEnabled is true and no triggers exist
      const userTz = getUserTimezone()
      let schedule: string
      if (scheduleMode === 'intervals') {
        schedule = `*/${selectedInterval} * * * *`
      } else {
        schedule = `CRON_TZ=${userTz} ${selectedMinute} ${selectedHour} * * ${selectedDays.join(',')}`
      }
      const newTriggers = [{ cron: { enabled: true, schedule, input: '' } }]
      onUpdate(newTriggers)
    }
  }, [defaultEnabled, hasCronTrigger, triggers.length, scheduleMode, selectedInterval, selectedDays, selectedHour, selectedMinute, selectedTimezone, onUpdate])

  const handleCronToggle = (enabled: boolean) => {
    if (enabled) {
      // Enable the existing cron trigger or create a default one if none exists
      const currentCronTrigger = triggers.find(t => t.cron)?.cron
      if (currentCronTrigger) {
        // Preserve existing configuration but set enabled to true
        const newTriggers = [...triggers.filter(t => !t.cron), { 
          cron: { 
            enabled: true, 
            schedule: currentCronTrigger.schedule || '', 
            input: currentCronTrigger.input || '' 
          } 
        }]
        onUpdate(newTriggers)
      } else {
        // Create a default cron trigger based on current mode
        const userTz = getUserTimezone()
        let schedule: string
        if (scheduleMode === 'intervals') {
          schedule = `*/${selectedInterval} * * * *`
        } else {
          schedule = `CRON_TZ=${userTz} 0 9 * * 1`
        }
        const newTriggers = [...triggers.filter(t => !t.cron), { cron: { enabled: true, schedule, input: '' } }]
        onUpdate(newTriggers)
      }
    } else {
      // Keep the cron trigger but set enabled to false, preserving schedule and input
      const currentCronTrigger = triggers.find(t => t.cron)?.cron
      if (currentCronTrigger) {
        const updatedTriggers = [...triggers.filter(t => !t.cron), { 
          cron: { 
            enabled: false, 
            schedule: currentCronTrigger.schedule || '', 
            input: currentCronTrigger.input || '' 
          } 
        }]
        onUpdate(updatedTriggers)
      } else {
        // Fallback: remove cron trigger if none exists
        const removedTriggers = triggers.filter(t => !t.cron)
        onUpdate(removedTriggers)
      }
    }
  }

  const handleScheduleModeChange = (mode: ScheduleMode) => {
    setScheduleMode(mode)
    if (mode === 'intervals') {
      // Switch to interval mode - create interval cron expression
      const schedule = `*/${selectedInterval} * * * *`
      const newTriggers = [...triggers.filter(t => !t.cron), { cron: { enabled: true, schedule, input: scheduleInput } }]
      onUpdate(newTriggers)
    } else {
      // Switch to specific time mode - create specific time cron expression
      const userTz = getUserTimezone()
      const schedule = `CRON_TZ=${userTz} ${selectedMinute} ${selectedHour} * * ${selectedDays.join(',')}`
      const newTriggers = [...triggers.filter(t => !t.cron), { cron: { enabled: true, schedule, input: scheduleInput } }]
      onUpdate(newTriggers)
    }
  }

  const handleDayToggle = (day: number) => {
    const newSelectedDays = selectedDays.includes(day) 
      ? selectedDays.filter(d => d !== day)
      : [...selectedDays, day].sort()
    
    setSelectedDays(newSelectedDays)
    updateCronTrigger(newSelectedDays, selectedHour, selectedMinute, selectedTimezone, scheduleInput)
  }

  const handleHourChange = (hour: number) => {
    setSelectedHour(hour)
    updateCronTrigger(selectedDays, hour, selectedMinute, selectedTimezone, scheduleInput)
  }

  const handleMinuteChange = (minute: number) => {
    setSelectedMinute(minute)
    updateCronTrigger(selectedDays, selectedHour, minute, selectedTimezone, scheduleInput)
  }

  const handleTimezoneChange = (timezone: string) => {
    setSelectedTimezone(timezone)
    updateCronTrigger(selectedDays, selectedHour, selectedMinute, timezone, scheduleInput)
  }

  const handleIntervalChange = (interval: number) => {
    setSelectedInterval(interval)
    updateIntervalCronTrigger(interval, scheduleInput)
  }

  const handleInputChange = (input: string) => {
    setScheduleInput(input)
    if (scheduleMode === 'intervals') {
      updateIntervalCronTrigger(selectedInterval, input)
    } else {
      updateCronTrigger(selectedDays, selectedHour, selectedMinute, selectedTimezone, input)
    }
  }

  const updateCronTrigger = (days: number[], hour: number, minute: number, timezone: string, input: string) => {
    if (days.length === 0) return
    
    const cronExpression = `CRON_TZ=${timezone} ${minute} ${hour} * * ${days.join(',')}`
    const newTriggers = [...triggers.filter(t => !t.cron), { cron: { enabled: true, schedule: cronExpression, input } }]
    onUpdate(newTriggers)
  }

  const updateIntervalCronTrigger = (interval: number, input: string) => {
    const cronExpression = `*/${interval} * * * *`
    const newTriggers = [...triggers.filter(t => !t.cron), { cron: { enabled: true, schedule: cronExpression, input } }]
    onUpdate(newTriggers)
  }

  return (
    <Box 
      sx={{ 
        p: 2, 
        borderRadius: borderRadius, 
        border: showBorder ? '1px solid' : 'none', 
        borderColor: 'divider' 
      }}
    >
      {showTitle && (
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
          {showToggle && (
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
          )}
        </Box>
      )}

      {(hasCronTrigger) && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1, opacity: hasCronTrigger ? 1 : 0.6 }}>
          {!hasCronTrigger && cronTrigger && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Trigger is disabled. Enable it above to activate the schedule.
            </Alert>
          )}
          {/* Schedule Mode Selection */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Schedule
            </Typography>
            <FormControl size="small" sx={{ minWidth: 200 }}>
              <Select
                value={scheduleMode}
                onChange={(e) => handleScheduleModeChange(e.target.value as ScheduleMode)}
                disabled={readOnly || !hasCronTrigger}
              >
                <MenuItem value="intervals">Intervals</MenuItem>
                <MenuItem value="specific_time">Specific Time</MenuItem>
              </Select>
            </FormControl>
          </Box>

          {scheduleMode === 'intervals' ? (
            /* Interval Mode */
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
                Interval
              </Typography>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <Select
                  value={selectedInterval}
                  onChange={(e) => handleIntervalChange(e.target.value as number)}
                  disabled={readOnly || !hasCronTrigger}
                >
                  {INTERVAL_MINUTES.map((interval) => (
                    <MenuItem key={interval} value={interval}>
                      {interval} minute{interval > 1 ? 's' : ''}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>
          ) : (
            /* Specific Time Mode */
            <Box sx={{ mb: 2 }}>
              {/* Days of the week */}
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
                Days of the week
              </Typography>
              <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'left' }}>
                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', mr: 2 }}>
                  {DAYS_OF_WEEK.map((day) => (
                    <Button
                      key={day.value}
                      variant={selectedDays.includes(day.value) ? "contained" : "outlined"}
                      color={selectedDays.includes(day.value) ? "secondary" : "primary"}
                      size="small"
                      onClick={() => handleDayToggle(day.value)}
                      disabled={readOnly || !hasCronTrigger}
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
                
                {/* Time selection - moved to the right side */}
                <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                  <FormControl size="small" sx={{ minWidth: 120 }}>
                    <InputLabel>Timezone</InputLabel>
                    <Select
                      value={selectedTimezone}
                      label="Timezone"
                      onChange={(e) => handleTimezoneChange(e.target.value as string)}
                      disabled={readOnly || !hasCronTrigger}
                    >
                      {timezones.map((timezone) => (
                        <MenuItem key={timezone} value={timezone}>
                          {timezone}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <FormControl size="small" sx={{ minWidth: 80 }}>
                    <InputLabel>Hour</InputLabel>
                    <Select
                      value={selectedHour}
                      label="Hour"
                      onChange={(e) => handleHourChange(e.target.value as number)}
                      disabled={readOnly || !hasCronTrigger}
                    >
                      {HOURS.map((hour) => (
                        <MenuItem key={hour} value={hour}>
                          {hour.toString().padStart(2, '0')}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Typography variant="body2" color="text.secondary">:</Typography>
                  <FormControl size="small" sx={{ minWidth: 80 }}>
                    <InputLabel>Minute</InputLabel>
                    <Select
                      value={selectedMinute}
                      label="Minute"
                      onChange={(e) => handleMinuteChange(e.target.value as number)}
                      disabled={readOnly || !hasCronTrigger}
                    >
                      {MINUTES.map((minute) => (
                        <MenuItem key={minute} value={minute}>
                          {minute.toString().padStart(2, '0')}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Box>
              </Box>
            </Box>
          )}

          {/* Input field */}
          <Box sx={{ mb: 2, mt: 4 }}>            
            <TextField
              fullWidth
              multiline
              rows={4}
              size="small"
              placeholder="Enter prompt here"
              value={scheduleInput}
              onChange={(e) => handleInputChange(e.target.value)}
              disabled={readOnly || !hasCronTrigger}
            />
          </Box>

          {/* Schedule summary */}
          <Box>
            <Typography variant="body2" color="text.secondary">
              <strong>Summary:</strong> {generateCronSummary(cronTrigger?.schedule || '')}
            </Typography>
          </Box>
        </Box>
      )}
    </Box>
  )
}

export default TriggerCron
