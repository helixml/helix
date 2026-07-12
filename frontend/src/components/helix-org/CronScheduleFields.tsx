// CronScheduleFields is the human-friendly schedule editor for cron
// topics (and anything else that stores a 5-field cron + optional
// CRON_TZ= prefix). Three modes:
//
//   - intervals:      every N minutes / hours
//   - specific_time:  weekday chips + hour:minute + timezone
//   - advanced:       free-form cron expression
//
// Mirrors the app-trigger UI in TriggerCron.tsx so operators see one
// mental model for "run on a schedule" across Helix.

import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { timezones } from '../../utils/timezones'
import {
  generateCronSummary,
  getUserTimezone,
  isIntervalCron,
  parseCronDays,
  parseCronHour,
  parseCronInterval,
  parseCronMinute,
  parseCronTimezone,
} from '../../utils/cronUtils'

export type CronScheduleMode = 'intervals' | 'specific_time' | 'advanced'

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

// Minimum backend interval is 90s — keep presets ≥ 2 minutes.
const INTERVAL_OPTIONS: Array<{ value: number; label: string }> = [
  { value: 2, label: 'Every 2 minutes' },
  { value: 5, label: 'Every 5 minutes' },
  { value: 10, label: 'Every 10 minutes' },
  { value: 15, label: 'Every 15 minutes' },
  { value: 30, label: 'Every 30 minutes' },
  { value: 60, label: 'Every hour' },
  { value: 120, label: 'Every 2 hours' },
  { value: 240, label: 'Every 4 hours' },
  { value: 360, label: 'Every 6 hours' },
  { value: 720, label: 'Every 12 hours' },
]

export function buildIntervalCron(intervalMinutes: number): string {
  return `*/${intervalMinutes} * * * *`
}

export function buildSpecificTimeCron(
  days: number[],
  hour: number,
  minute: number,
  timezone: string,
): string {
  const ordered = [...days].sort((a, b) => a - b)
  return `CRON_TZ=${timezone} ${minute} ${hour} * * ${ordered.join(',')}`
}

/** Infer the friendliest mode for a stored cron string. */
export function inferCronMode(schedule: string): CronScheduleMode {
  const s = (schedule || '').trim()
  if (!s) return 'specific_time'
  if (isIntervalCron(s)) return 'intervals'
  // Specific-time form we generate: optional CRON_TZ= + "M H * * days"
  const bare = s.startsWith('CRON_TZ=') ? s.replace(/^CRON_TZ=\S+\s+/, '') : s
  const parts = bare.split(/\s+/)
  if (
    parts.length === 5 &&
    parts[2] === '*' &&
    parts[3] === '*' &&
    !parts[0].includes('/') &&
    !parts[1].includes('/')
  ) {
    return 'specific_time'
  }
  return 'advanced'
}

export type CronScheduleFieldsProps = {
  /** Current cron schedule (what gets stored on the topic). */
  value: string
  onChange: (schedule: string) => void
  disabled?: boolean
  /** Seed mode when value is empty (create form). */
  defaultMode?: CronScheduleMode
}

const CronScheduleFields: FC<CronScheduleFieldsProps> = ({
  value,
  onChange,
  disabled = false,
  defaultMode = 'specific_time',
}) => {
  const [mode, setMode] = useState<CronScheduleMode>(() =>
    value.trim() ? inferCronMode(value) : defaultMode,
  )
  const [selectedDays, setSelectedDays] = useState<number[]>(() => {
    if (value.trim() && !isIntervalCron(value)) {
      const d = parseCronDays(value)
      return d.length > 0 ? d : [1, 2, 3, 4, 5]
    }
    return [1, 2, 3, 4, 5]
  })
  const [selectedHour, setSelectedHour] = useState<number>(() =>
    value.trim() && !isIntervalCron(value) ? parseCronHour(value) : 9,
  )
  const [selectedMinute, setSelectedMinute] = useState<number>(() =>
    value.trim() && !isIntervalCron(value) ? parseCronMinute(value) : 0,
  )
  const [selectedTimezone, setSelectedTimezone] = useState<string>(() =>
    value.trim() && !isIntervalCron(value) ? parseCronTimezone(value) : getUserTimezone(),
  )
  const [selectedInterval, setSelectedInterval] = useState<number>(() =>
    value.trim() && isIntervalCron(value) ? parseCronInterval(value) : 15,
  )
  const [advancedText, setAdvancedText] = useState<string>(value || '0 9 * * 1-5')

  // Parent remounts this component (key=…) when the create drawer reopens
  // so local state always matches the seed `value` without fighting live edits.

  const emitInterval = (interval: number) => {
    onChange(buildIntervalCron(interval))
  }

  const emitSpecific = (days: number[], hour: number, minute: number, tz: string) => {
    if (days.length === 0) {
      onChange('')
      return
    }
    onChange(buildSpecificTimeCron(days, hour, minute, tz))
  }

  const handleModeChange = (next: CronScheduleMode) => {
    setMode(next)
    if (next === 'intervals') {
      emitInterval(selectedInterval)
    } else if (next === 'specific_time') {
      emitSpecific(selectedDays, selectedHour, selectedMinute, selectedTimezone)
    } else {
      // Seed advanced from whatever we currently have, if any.
      const seed = value.trim() || advancedText || '0 9 * * 1-5'
      setAdvancedText(seed)
      onChange(seed)
    }
  }

  const handleDayToggle = (day: number) => {
    const next = selectedDays.includes(day)
      ? selectedDays.filter((d) => d !== day)
      : [...selectedDays, day].sort((a, b) => a - b)
    setSelectedDays(next)
    emitSpecific(next, selectedHour, selectedMinute, selectedTimezone)
  }

  const summary = useMemo(() => {
    if (!value.trim()) {
      if (mode === 'specific_time' && selectedDays.length === 0) return 'Pick at least one day'
      return 'No schedule'
    }
    if (mode === 'advanced') {
      // Free-form may not parse as interval/specific; still try summary.
      try {
        return generateCronSummary(value)
      } catch {
        return value
      }
    }
    return generateCronSummary(value)
  }, [value, mode, selectedDays.length])

  return (
    <Stack spacing={1.5}>
      <FormControl fullWidth size="small">
        <InputLabel id="cron-mode-label">Schedule type</InputLabel>
        <Select
          labelId="cron-mode-label"
          label="Schedule type"
          value={mode}
          onChange={(e) => handleModeChange(e.target.value as CronScheduleMode)}
          disabled={disabled}
        >
          <MenuItem value="specific_time">Specific time (weekdays + hour)</MenuItem>
          <MenuItem value="intervals">Interval (every …)</MenuItem>
          <MenuItem value="advanced">Advanced (cron expression)</MenuItem>
        </Select>
      </FormControl>

      {mode === 'intervals' && (
        <FormControl fullWidth size="small">
          <InputLabel id="cron-interval-label">How often</InputLabel>
          <Select
            labelId="cron-interval-label"
            label="How often"
            value={selectedInterval}
            onChange={(e) => {
              const n = e.target.value as number
              setSelectedInterval(n)
              emitInterval(n)
            }}
            disabled={disabled}
          >
            {INTERVAL_OPTIONS.map((opt) => (
              <MenuItem key={opt.value} value={opt.value}>
                {opt.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      )}

      {mode === 'specific_time' && (
        <Stack spacing={1.5}>
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.75 }}>
              Days of the week
            </Typography>
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {DAYS_OF_WEEK.map((day) => (
                <Button
                  key={day.value}
                  variant={selectedDays.includes(day.value) ? 'contained' : 'outlined'}
                  color={selectedDays.includes(day.value) ? 'secondary' : 'primary'}
                  size="small"
                  onClick={() => handleDayToggle(day.value)}
                  disabled={disabled}
                  sx={{ minWidth: 'auto', px: 1.25, py: 0.5, fontSize: '0.75rem' }}
                >
                  {day.label}
                </Button>
              ))}
            </Stack>
            {selectedDays.length === 0 && (
              <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
                Select at least one day.
              </Typography>
            )}
          </Box>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <FormControl size="small" sx={{ minWidth: 160, flex: 1 }}>
              <InputLabel>Timezone</InputLabel>
              <Select
                label="Timezone"
                value={selectedTimezone}
                onChange={(e) => {
                  const tz = e.target.value as string
                  setSelectedTimezone(tz)
                  emitSpecific(selectedDays, selectedHour, selectedMinute, tz)
                }}
                disabled={disabled}
              >
                {timezones.map((tz) => (
                  <MenuItem key={tz} value={tz}>
                    {tz}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 88 }}>
              <InputLabel>Hour</InputLabel>
              <Select
                label="Hour"
                value={selectedHour}
                onChange={(e) => {
                  const h = e.target.value as number
                  setSelectedHour(h)
                  emitSpecific(selectedDays, h, selectedMinute, selectedTimezone)
                }}
                disabled={disabled}
              >
                {HOURS.map((h) => (
                  <MenuItem key={h} value={h}>
                    {h.toString().padStart(2, '0')}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <Typography variant="body2" color="text.secondary">
              :
            </Typography>
            <FormControl size="small" sx={{ minWidth: 88 }}>
              <InputLabel>Minute</InputLabel>
              <Select
                label="Minute"
                value={selectedMinute}
                onChange={(e) => {
                  const m = e.target.value as number
                  setSelectedMinute(m)
                  emitSpecific(selectedDays, selectedHour, m, selectedTimezone)
                }}
                disabled={disabled}
              >
                {MINUTES.map((m) => (
                  <MenuItem key={m} value={m}>
                    {m.toString().padStart(2, '0')}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Stack>
        </Stack>
      )}

      {mode === 'advanced' && (
        <TextField
          label="Cron expression"
          placeholder="0 9 * * 1-5"
          value={advancedText}
          onChange={(e) => {
            setAdvancedText(e.target.value)
            onChange(e.target.value)
          }}
          fullWidth
          size="small"
          helperText="Standard 5-field cron: minute hour day-of-month month day-of-week. Prefix with CRON_TZ=<zone> to pin the timezone (defaults to UTC). Minimum interval: 90 seconds."
          sx={{ '& input': { fontFamily: 'monospace', fontSize: '0.85rem' } }}
          disabled={disabled}
        />
      )}

      <Box
        sx={{
          px: 1.5,
          py: 1,
          borderRadius: 1,
          border: '1px solid rgba(0,0,0,0.08)',
          bgcolor: 'rgba(0,0,0,0.02)',
        }}
      >
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
          <strong>Summary:</strong> {summary}
        </Typography>
        {mode !== 'advanced' && value.trim() && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', mt: 0.25, fontFamily: 'monospace' }}
          >
            {value}
          </Typography>
        )}
      </Box>
    </Stack>
  )
}

export default CronScheduleFields
