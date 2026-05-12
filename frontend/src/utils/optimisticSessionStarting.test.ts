import { describe, it, expect, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { optimisticallyMarkSessionStarting } from './optimisticSessionStarting'
import { GET_SESSION_QUERY_KEY } from '../services/sessionService'

const SESSION_ID = 'ses_01test'
const fullKey = [...GET_SESSION_QUERY_KEY(SESSION_ID), 'full']
const skipKey = [...GET_SESSION_QUERY_KEY(SESSION_ID), 'skip']

const seed = (qc: QueryClient, status: string | undefined, variant: 'full' | 'skip' = 'full') => {
  qc.setQueryData(
    [...GET_SESSION_QUERY_KEY(SESSION_ID), variant],
    {
      data: {
        id: SESSION_ID,
        config: status === undefined ? {} : { external_agent_status: status },
      },
    },
  )
}

describe('optimisticallyMarkSessionStarting', () => {
  it('flips paused (no status) to "starting" in both full and skip slots', () => {
    const qc = new QueryClient()
    seed(qc, undefined, 'full')
    seed(qc, undefined, 'skip')
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(
      (qc.getQueryData(fullKey) as { data: { config: { external_agent_status: string } } }).data.config.external_agent_status,
    ).toBe('starting')
    expect(
      (qc.getQueryData(skipKey) as { data: { config: { external_agent_status: string } } }).data.config.external_agent_status,
    ).toBe('starting')
  })

  it('flips status="absent" to "starting"', () => {
    const qc = new QueryClient()
    seed(qc, 'absent', 'full')
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(
      (qc.getQueryData(fullKey) as { data: { config: { external_agent_status: string } } }).data.config.external_agent_status,
    ).toBe('starting')
  })

  it('no-op when status is already "starting"', () => {
    const qc = new QueryClient()
    seed(qc, 'starting', 'full')
    const before = qc.getQueryData(fullKey)
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(qc.getQueryData(fullKey)).toBe(before)
  })

  it('no-op when status is "running"', () => {
    const qc = new QueryClient()
    seed(qc, 'running', 'full')
    const before = qc.getQueryData(fullKey)
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(qc.getQueryData(fullKey)).toBe(before)
  })

  it('does not seed an entry when the slot is empty', () => {
    const qc = new QueryClient()
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(qc.getQueryData(fullKey)).toBeUndefined()
    expect(qc.getQueryData(skipKey)).toBeUndefined()
  })

  it('seeds a default status_message when none exists', () => {
    const qc = new QueryClient()
    seed(qc, 'absent', 'full')
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    const cfg = (qc.getQueryData(fullKey) as { data: { config: { status_message: string } } }).data.config
    expect(cfg.status_message).toBe('Starting Desktop...')
  })

  it('preserves an existing status_message', () => {
    const qc = new QueryClient()
    qc.setQueryData(fullKey, {
      data: {
        id: SESSION_ID,
        config: {
          external_agent_status: 'absent',
          status_message: 'Unpacking build cache...',
        },
      },
    })
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    const cfg = (qc.getQueryData(fullKey) as { data: { config: { status_message: string } } }).data.config
    expect(cfg.status_message).toBe('Unpacking build cache...')
  })

  it('preserves unrelated session fields', () => {
    const qc = new QueryClient()
    qc.setQueryData(fullKey, {
      data: {
        id: SESSION_ID,
        name: 'My session',
        interactions: [{ id: 'int_1' }],
        config: { external_agent_status: 'absent', container_name: 'box-42' },
      },
    })
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    const data = (qc.getQueryData(fullKey) as {
      data: {
        id: string
        name: string
        interactions: Array<{ id: string }>
        config: { external_agent_status: string; container_name: string }
      }
    }).data
    expect(data.id).toBe(SESSION_ID)
    expect(data.name).toBe('My session')
    expect(data.interactions).toEqual([{ id: 'int_1' }])
    expect(data.config.container_name).toBe('box-42')
    expect(data.config.external_agent_status).toBe('starting')
  })

  it('fires invalidateQueries on the prefix key to nudge the next poll', () => {
    const qc = new QueryClient()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    optimisticallyMarkSessionStarting(qc, SESSION_ID)
    expect(spy).toHaveBeenCalledWith({
      queryKey: GET_SESSION_QUERY_KEY(SESSION_ID),
    })
  })

  it('returns immediately on empty sessionId', () => {
    const qc = new QueryClient()
    const spy = vi.spyOn(qc, 'setQueryData')
    optimisticallyMarkSessionStarting(qc, '')
    expect(spy).not.toHaveBeenCalled()
  })
})
