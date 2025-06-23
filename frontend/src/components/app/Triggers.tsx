import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Grid from '@mui/material/Grid'
import Chip from '@mui/material/Chip'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import FormGroup from '@mui/material/FormGroup'
import Checkbox from '@mui/material/Checkbox'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import EditIcon from '@mui/icons-material/Edit'
import ScheduleIcon from '@mui/icons-material/Schedule'
import ApiIcon from '@mui/icons-material/Api'
import { TypesTrigger, TypesCronTrigger } from '../../api/api'
import useThemeConfig from '../../hooks/useThemeConfig'

interface TriggersProps {
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

interface CronScheduleDialogProps {
  open: boolean
  onClose: () => void
  onSave: (schedule: TypesCronTrigger) => void
  initialSchedule?: TypesCronTrigger
}

const DAYS_OF_WEEK = [
  { value: 0, label: 'Sunday' },
  { value: 1, label: 'Monday' },
  { value: 2, label: 'Tuesday' },
  { value: 3, label: 'Wednesday' },
  { value: 4, label: 'Thursday' },
  { value: 5, label: 'Friday' },
  { value: 6, label: 'Saturday' },
]

const HOURS = Array.from({ length: 24 }, (_, i) => i)
const MINUTES = Array.from({ length: 60 }, (_, i) => i)

const CronScheduleDialog: FC<CronScheduleDialogProps> = ({
  open,
  onClose,
  onSave,
  initialSchedule
}) => {
  const [selectedDays, setSelectedDays] = useState<number[]>(initialSchedule ? parseCronDays(initialSchedule.schedule || '') : [])
  const [selectedHour, setSelectedHour] = useState<number>(initialSchedule ? parseCronHour(initialSchedule.schedule || '') : 9)
  const [selectedMinute, setSelectedMinute] = useState<number>(initialSchedule ? parseCronMinute(initialSchedule.schedule || '') : 0)
  const [input, setInput] = useState<string>(initialSchedule?.input || '')

  const handleDayToggle = (day: number) => {
    setSelectedDays(prev => 
      prev.includes(day) 
        ? prev.filter(d => d !== day)
        : [...prev, day].sort()
    )
  }

  const handleSave = () => {
    if (selectedDays.length === 0) return
    
    // Standard cron format: minute hour day month weekday
    // For weekly schedules, we use 0 for day and month (meaning "every day/month")
    // and specify the weekday(s) in the last field
    const cronExpression = `${selectedMinute} ${selectedHour} * * ${selectedDays.join(',')}`
    onSave({
      schedule: cronExpression,
      input: input
    })
    onClose()
  }

  const isValid = selectedDays.length > 0

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Configure Recurring Schedule</DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <Typography variant="h6" gutterBottom>
            Days of the Week
          </Typography>
          <FormGroup row>
            {DAYS_OF_WEEK.map((day) => (
              <FormControlLabel
                key={day.value}
                control={
                  <Checkbox
                    checked={selectedDays.includes(day.value)}
                    onChange={() => handleDayToggle(day.value)}
                  />
                }
                label={day.label}
              />
            ))}
          </FormGroup>

          <Box sx={{ mt: 3 }}>
            <Typography variant="h6" gutterBottom>
              Time
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={6}>
                <FormControl fullWidth>
                  <InputLabel>Hour</InputLabel>
                  <Select
                    value={selectedHour}
                    label="Hour"
                    onChange={(e) => setSelectedHour(e.target.value as number)}
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
                <FormControl fullWidth>
                  <InputLabel>Minute</InputLabel>
                  <Select
                    value={selectedMinute}
                    label="Minute"
                    onChange={(e) => setSelectedMinute(e.target.value as number)}
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

          <Box sx={{ mt: 3 }}>
            <Typography variant="h6" gutterBottom>
              Input (Optional)
            </Typography>
            <TextField
              fullWidth
              multiline
              rows={3}
              placeholder="Enter the input that will be sent to the agent when triggered..."
              value={input}
              onChange={(e) => setInput(e.target.value)}
            />
          </Box>

          <Box sx={{ mt: 2 }}>
            <Typography variant="body2" color="text.secondary">
              Schedule: {selectedDays.length > 0 ? `Every ${selectedDays.map(d => DAYS_OF_WEEK[d].label).join(', ')} at ${selectedHour.toString().padStart(2, '0')}:${selectedMinute.toString().padStart(2, '0')}` : 'No days selected'}
            </Typography>
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave} variant="contained" disabled={!isValid}>
          Save Schedule
        </Button>
      </DialogActions>
    </Dialog>
  )
}

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
  
  const dayLabels = days.map(d => DAYS_OF_WEEK[d].label)
  return `Every ${dayLabels.join(', ')} at ${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}`
}

const Triggers: FC<TriggersProps> = ({
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const [cronDialogOpen, setCronDialogOpen] = useState(false)
  const [editingCronIndex, setEditingCronIndex] = useState<number | null>(null)

  const hasCronTrigger = triggers.some(t => t.cron)
  const cronTrigger = triggers.find(t => t.cron)?.cron

  const themeConfig = useThemeConfig()

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

  const handleCronEdit = (index: number) => {
    setEditingCronIndex(index)
    setCronDialogOpen(true)
  }

  const handleCronSave = (schedule: TypesCronTrigger) => {
    const newTriggers = [...triggers]
    const cronIndex = newTriggers.findIndex(t => t.cron)
    
    if (cronIndex >= 0) {
      newTriggers[cronIndex] = { cron: schedule }
    } else {
      newTriggers.push({ cron: schedule })
    }
    
    onUpdate(newTriggers)
  }

  const handleCronDelete = () => {
    const newTriggers = triggers.filter(t => !t.cron)
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
      <Card sx={{ mb: 3, backgroundColor: themeConfig.darkPanel, boxShadow: 'none' }}>
        <CardContent>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <ApiIcon sx={{ mr: 2, color: 'primary.main' }} />
              <Box>
                <Typography variant="h6">Sessions & API</Typography>
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
        </CardContent>
      </Card>

      {/* Recurring Trigger */}
      <Card sx={{ backgroundColor: themeConfig.darkPanel, boxShadow: 'none' }}>
        <CardContent>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <ScheduleIcon sx={{ mr: 2, color: 'primary.main' }} />
              <Box>
                <Typography variant="h6">Scheduled</Typography>
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

          {hasCronTrigger && cronTrigger && (
            <Box sx={{ mt: 2, p: 2, borderRadius: 1 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Box>
                  <Typography variant="subtitle1" gutterBottom>
                    Schedule
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {formatCronSchedule(cronTrigger.schedule || '')}
                  </Typography>
                  {cronTrigger.input && (
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                      Input: {cronTrigger.input}
                    </Typography>
                  )}
                </Box>
                <IconButton
                  onClick={() => handleCronEdit(0)}
                  disabled={readOnly}
                  size="small"
                >
                  <EditIcon />
                </IconButton>
              </Box>
            </Box>
          )}
        </CardContent>
      </Card>

      <CronScheduleDialog
        open={cronDialogOpen}
        onClose={() => {
          setCronDialogOpen(false)
          setEditingCronIndex(null)
        }}
        onSave={handleCronSave}
        initialSchedule={editingCronIndex !== null ? triggers[editingCronIndex]?.cron : undefined}
      />
    </Box>
  )
}

export default Triggers
