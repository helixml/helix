export const toUsageDateInput = (date: Date) => date.toISOString().slice(0, 10)

export const usageRangeFrom = (days: number, now = new Date()) => {
  const from = new Date(now)
  from.setDate(from.getDate() - (days - 1))
  return toUsageDateInput(from)
}

export const buildTaskUsageQuery = (taskId: string, now = new Date()) => ({
  from: usageRangeFrom(7, now),
  to: toUsageDateInput(now),
  task_id: taskId,
})

export const initialUsageParam = (
  routeParams: Record<string, unknown>,
  searchParams: URLSearchParams,
  key: string,
) => {
  const routeValue = routeParams[key]
  return typeof routeValue === 'string' ? routeValue : searchParams.get(key) || ''
}
