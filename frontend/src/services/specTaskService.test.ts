import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'

import { invalidateSpecTaskStatusQueries } from './specTaskService'

describe('invalidateSpecTaskStatusQueries', () => {
  it('invalidates the active task and all task lists without touching unrelated queries', async () => {
    const queryClient = new QueryClient()
    const taskKey = ['spec-tasks', 'task-one'] as const
    const firstListKey = ['spec-tasks', 'list', { projectId: 'project-one' }] as const
    const secondListKey = ['spec-tasks', 'list', { projectId: 'project-two' }] as const
    const unrelatedKey = ['sessions', 'project-one'] as const

    queryClient.setQueryData(taskKey, { id: 'task-one' })
    queryClient.setQueryData(firstListKey, [{ id: 'task-one' }])
    queryClient.setQueryData(secondListKey, [{ id: 'task-two' }])
    queryClient.setQueryData(unrelatedKey, [])

    await invalidateSpecTaskStatusQueries(queryClient, 'task-one')

    expect(queryClient.getQueryState(taskKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(firstListKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(secondListKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(unrelatedKey)?.isInvalidated).toBe(false)
  })
})
