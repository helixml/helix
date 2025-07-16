// Helper functions to parse cron expressions and generate human-readable summaries

const DAYS_OF_WEEK = [
  { value: 0, label: 'SUN', fullLabel: 'Sunday' },
  { value: 1, label: 'MON', fullLabel: 'Monday' },
  { value: 2, label: 'TUE', fullLabel: 'Tuesday' },
  { value: 3, label: 'WED', fullLabel: 'Wednesday' },
  { value: 4, label: 'THU', fullLabel: 'Thursday' },
  { value: 5, label: 'FRI', fullLabel: 'Friday' },
  { value: 6, label: 'SAT', fullLabel: 'Saturday' },
]

// Parse cron expression to extract days of the week
export const parseCronDays = (cronExpression: string): number[] => {
  const parts = cronExpression.split(' ')
  // Check if it has timezone prefix
  const hasTimezone = cronExpression.startsWith('CRON_TZ=')
  const dayIndex = hasTimezone ? 5 : 4 // weekday field is shifted by 1 if timezone is present
  
  if (parts.length >= (hasTimezone ? 6 : 5)) {
    const dayPart = parts[dayIndex]
    return dayPart.split(',').map(d => parseInt(d)).filter(d => !isNaN(d))
  }
  return []
}

// Parse cron expression to extract hour
export const parseCronHour = (cronExpression: string): number => {
  const parts = cronExpression.split(' ')
  // Check if it has timezone prefix
  const hasTimezone = cronExpression.startsWith('CRON_TZ=')
  const hourIndex = hasTimezone ? 2 : 1 // hour field is shifted by 1 if timezone is present
  
  if (parts.length >= (hasTimezone ? 6 : 5)) {
    return parseInt(parts[hourIndex]) || 9
  }
  return 9
}

// Parse cron expression to extract minute
export const parseCronMinute = (cronExpression: string): number => {
  const parts = cronExpression.split(' ')
  // Check if it has timezone prefix
  const hasTimezone = cronExpression.startsWith('CRON_TZ=')
  const minuteIndex = hasTimezone ? 1 : 0 // minute field is shifted by 1 if timezone is present
  
  if (parts.length >= (hasTimezone ? 6 : 5)) {
    return parseInt(parts[minuteIndex]) || 0
  }
  return 0
}

// Parse cron expression to extract timezone
export const parseCronTimezone = (cronExpression: string): string => {
  // Check if the cron expression starts with CRON_TZ=
  if (cronExpression.startsWith('CRON_TZ=')) {
    const tzMatch = cronExpression.match(/^CRON_TZ=([^\s]+)\s/)
    if (tzMatch) {
      return tzMatch[1]
    }
  }
  return getUserTimezone() // Use user's timezone as default instead of hardcoded UTC
}

// Parse cron expression to extract interval (for interval mode)
export const parseCronInterval = (cronExpression: string): number => {
  const parts = cronExpression.split(' ')
  // For interval mode, the expression should be like "*/5 * * * *" (every 5 minutes)
  if (parts.length >= 5) {
    const minutePart = parts[0]
    if (minutePart.startsWith('*/')) {
      return parseInt(minutePart.substring(2)) || 5
    }
  }
  return 5
}

// Check if cron expression is in interval mode
export const isIntervalCron = (cronExpression: string): boolean => {
  const parts = cronExpression.split(' ')
  if (parts.length >= 5) {
    const minutePart = parts[0]
    return minutePart.startsWith('*/')
  }
  return false
}

// Get user's timezone
export const getUserTimezone = () => {
  const userTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return timezones.includes(userTimezone) ? userTimezone : 'UTC'
}

// Import timezones from the existing utils
import { timezones } from './timezones'

// Generate a human-readable summary of a cron expression
export const generateCronSummary = (cronExpression: string): string => {
  if (!cronExpression) return 'No schedule'
  
  // Check if it's an interval schedule
  if (isIntervalCron(cronExpression)) {
    const interval = parseCronInterval(cronExpression)
    return `Every ${interval} minute${interval > 1 ? 's' : ''}`
  }
  
  // Check if it's a specific time schedule
  const days = parseCronDays(cronExpression)
  const hour = parseCronHour(cronExpression)
  const minute = parseCronMinute(cronExpression)
  const timezone = parseCronTimezone(cronExpression)
  
  if (days.length === 0) {
    return 'No days selected'
  }
  
  const timeStr = `${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}`
  const dayLabels = days.map(d => DAYS_OF_WEEK[d]?.fullLabel || `Day ${d}`).join(', ')
  
  return `Every ${dayLabels} at ${timeStr} (${timezone})`
}

// Generate a short summary for display in lists (like Home.tsx)
export const generateCronShortSummary = (cronExpression: string): string => {
  if (!cronExpression) return 'No schedule'
  
  // Check if it's an interval schedule
  if (isIntervalCron(cronExpression)) {
    const interval = parseCronInterval(cronExpression)
    if (interval === 5) return 'Every 5 minutes'
    if (interval === 10) return 'Every 10 minutes'
    if (interval === 15) return 'Every 15 minutes'
    if (interval === 30) return 'Every 30 minutes'
    if (interval === 60) return 'Every hour'
    if (interval === 120) return 'Every 2 hours'
    if (interval === 240) return 'Every 4 hours'
    return `Every ${interval} minutes`
  }
  
  // Check if it's a specific time schedule
  const days = parseCronDays(cronExpression)
  const hour = parseCronHour(cronExpression)
  const minute = parseCronMinute(cronExpression)
  
  if (days.length === 0) {
    return 'No schedule'
  }
  
  const timeStr = `${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}`
  
  // Handle common day patterns
  if (days.length === 7) {
    return `Daily at ${timeStr}`
  } else if (days.length === 5 && days.every(d => d >= 1 && d <= 5)) {
    return `Weekdays at ${timeStr}`
  } else if (days.length === 2 && days.includes(0) && days.includes(6)) {
    return `Weekends at ${timeStr}`
  } else if (days.length === 1) {
    const dayName = DAYS_OF_WEEK[days[0]]?.fullLabel || `Day ${days[0]}`
    return `${dayName} at ${timeStr}`
  } else {
    const dayNames = days.map(d => DAYS_OF_WEEK[d]?.label || `D${d}`).join(', ')
    return `${dayNames} at ${timeStr}`
  }
} 