export type BotTaskStats = {
  backlog: number
  inProgress: number
  done: number
}

const IN_PROGRESS_STATUSES = new Set([
  'queued_spec_generation',
  'spec_generation',
  'spec_review',
  'spec_revision',
  'spec_approved',
  'queued_implementation',
  'implementation_queued',
  'implementation',
  'implementation_review',
  'pull_request',
  // Legacy status values still returned by older task records.
  'planning',
  'review',
  'implementing',
])

export const summarizeBotTasks = (tasks: Array<{ status?: string | null }>): BotTaskStats =>
  tasks.reduce<BotTaskStats>((stats, task) => {
    const status = task.status ?? ''
    if (status === 'backlog') stats.backlog += 1
    else if (status === 'done' || status === 'completed') stats.done += 1
    else if (IN_PROGRESS_STATUSES.has(status)) stats.inProgress += 1
    return stats
  }, { backlog: 0, inProgress: 0, done: 0 })
