import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  onSessionExpired,
  runSessionCheck,
  isUnauthorizedApiError,
  __resetSessionExpiryForTests,
} from './sessionExpiry'

afterEach(() => __resetSessionExpiryForTests())

describe('runSessionCheck', () => {
  it('notifies listeners only when the server says not authenticated', async () => {
    const cb = vi.fn()
    onSessionExpired(cb)

    await runSessionCheck(async () => true)
    await runSessionCheck(async () => undefined)
    await runSessionCheck(async () => { throw new Error('network') })
    expect(cb).not.toHaveBeenCalled()

    await runSessionCheck(async () => false)
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('shares one probe across concurrent callers', async () => {
    const cb = vi.fn()
    onSessionExpired(cb)
    let resolve!: (v: boolean) => void
    const probe = vi.fn(() => new Promise<boolean>((r) => { resolve = r }))

    const checks = [runSessionCheck(probe), runSessionCheck(probe), runSessionCheck(probe)]
    resolve(false)
    await Promise.all(checks)

    expect(probe).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('stops notifying after unsubscribe', async () => {
    const cb = vi.fn()
    const unsubscribe = onSessionExpired(cb)
    unsubscribe()
    await runSessionCheck(async () => false)
    expect(cb).not.toHaveBeenCalled()
  })
})

describe('isUnauthorizedApiError', () => {
  it('matches 401s from regular endpoints only', () => {
    expect(isUnauthorizedApiError({ response: { status: 401 }, config: { url: '/api/v1/projects' } })).toBe(true)
    expect(isUnauthorizedApiError({ response: { status: 401 }, config: { url: '/api/v1/auth/user' } })).toBe(false)
    expect(isUnauthorizedApiError({ response: { status: 403 }, config: { url: '/api/v1/projects' } })).toBe(false)
    expect(isUnauthorizedApiError({ response: { status: 503 }, config: { url: '/api/v1/projects' } })).toBe(false)
    expect(isUnauthorizedApiError(new Error('network'))).toBe(false)
  })
})
