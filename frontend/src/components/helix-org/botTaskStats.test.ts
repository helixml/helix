import { describe, expect, it } from 'vitest'

import { summarizeBotTasks } from './botTaskStats'

describe('summarizeBotTasks', () => {
  it('groups backlog, active workflow, and completed tasks', () => {
    expect(summarizeBotTasks([
      { status: 'backlog' },
      { status: 'spec_generation' },
      { status: 'spec_review' },
      { status: 'implementation' },
      { status: 'implementation_review' },
      { status: 'done' },
    ])).toEqual({ backlog: 1, inProgress: 4, done: 1 })
  })

  it('does not report failed tasks as in progress', () => {
    expect(summarizeBotTasks([
      { status: 'spec_failed' },
      { status: 'implementation_failed' },
    ])).toEqual({ backlog: 0, inProgress: 0, done: 0 })
  })
})
