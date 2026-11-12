import { describe, expect, it, vi } from 'vitest'
import { createSessionExpiryHandler, isSignInExemptRoute } from './sessionExpiry'

const unauthorized = (url = '/api/v1/sessions') => ({ response: { status: 401 }, config: { url } })

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

function setup(answer: boolean | null) {
  let time = 0
  const checkAuthenticated = vi.fn(async () => answer)
  const redirectToLogin = vi.fn()
  const handler = createSessionExpiryHandler(
    { checkAuthenticated, redirectToLogin, recheckAfterMs: 30_000, now: () => time },
    'https://app.example.test',
  )
  const advance = (ms: number) => { time += ms }
  return { handler, checkAuthenticated, redirectToLogin, advance }
}

const fail = (handler: ReturnType<typeof createSessionExpiryHandler>, error: any) =>
  handler.onResponseError(error).catch(() => undefined)

describe('session expiry handler', () => {
  it('sends a signed-in user to login once when the session has ended', async () => {
    const { handler, checkAuthenticated, redirectToLogin } = setup(false)
    handler.setSignedIn(true)

    // A burst of polling requests all failing with 401.
    await Promise.all(Array.from({ length: 10 }, () => fail(handler, unauthorized())))
    await flush()
    await Promise.all(Array.from({ length: 10 }, () => fail(handler, unauthorized())))
    await flush()

    expect(checkAuthenticated).toHaveBeenCalledTimes(1)
    expect(redirectToLogin).toHaveBeenCalledTimes(1)
  })

  it('rejects with the original error so callers still see the failure', async () => {
    const { handler } = setup(false)
    handler.setSignedIn(true)
    const error = unauthorized()
    await expect(handler.onResponseError(error)).rejects.toBe(error)
  })

  it('stays put when the session is still valid, and does not re-check on every 401', async () => {
    const { handler, checkAuthenticated, redirectToLogin, advance } = setup(true)
    handler.setSignedIn(true)

    await fail(handler, unauthorized())
    await flush()
    await fail(handler, unauthorized())
    await flush()
    expect(checkAuthenticated).toHaveBeenCalledTimes(1)

    advance(30_000)
    await fail(handler, unauthorized())
    await flush()
    expect(checkAuthenticated).toHaveBeenCalledTimes(2)
    expect(redirectToLogin).not.toHaveBeenCalled()
  })

  it('does nothing when the check cannot tell', async () => {
    const { handler, redirectToLogin } = setup(null)
    handler.setSignedIn(true)
    await fail(handler, unauthorized())
    await flush()
    expect(redirectToLogin).not.toHaveBeenCalled()
  })

  it('ignores 401s before sign-in, from non-API URLs, and other statuses', async () => {
    const { handler, checkAuthenticated } = setup(false)
    await fail(handler, unauthorized())
    handler.setSignedIn(true)
    await fail(handler, unauthorized('https://other.example/api/v1/x'))
    await fail(handler, { response: { status: 403 }, config: { url: '/api/v1/x' } })
    await fail(handler, { response: { status: 500 }, config: { url: '/api/v1/x' } })
    await flush()
    expect(checkAuthenticated).not.toHaveBeenCalled()

    await fail(handler, unauthorized('https://app.example.test/api/v1/x'))
    await flush()
    expect(checkAuthenticated).toHaveBeenCalledTimes(1)
  })

  it.each(['embed_session', 'embed_spec_task', 'login', 'password-reset', 'password-reset-complete', 'artifact_viewer'])(
    'never redirects from the %s route',
    async (routeName) => {
      const { handler, checkAuthenticated, redirectToLogin } = setup(false)
      handler.setSignedIn(true)
      handler.setRouteName(routeName)
      await fail(handler, unauthorized())
      await flush()
      expect(checkAuthenticated).not.toHaveBeenCalled()
      expect(redirectToLogin).not.toHaveBeenCalled()
    },
  )

  it('does not redirect if the user moved to an exempt route while the check was running', async () => {
    let resolve: (v: boolean) => void = () => {}
    const checkAuthenticated = vi.fn(() => new Promise<boolean>(r => { resolve = r }))
    const redirectToLogin = vi.fn()
    const handler = createSessionExpiryHandler({ checkAuthenticated, redirectToLogin }, '')
    handler.setSignedIn(true)
    handler.setRouteName('org_projects')
    await fail(handler, unauthorized())
    handler.setRouteName('login')
    resolve(false)
    await flush()
    expect(checkAuthenticated).toHaveBeenCalledTimes(1)
    expect(redirectToLogin).not.toHaveBeenCalled()
  })
})

describe('isSignInExemptRoute', () => {
  it('matches the routes that render without an account', () => {
    expect(isSignInExemptRoute('embed_anything')).toBe(true)
    expect(isSignInExemptRoute('artifact_viewer')).toBe(true)
    expect(isSignInExemptRoute('org_projects')).toBe(false)
    expect(isSignInExemptRoute(undefined)).toBe(false)
  })
})
