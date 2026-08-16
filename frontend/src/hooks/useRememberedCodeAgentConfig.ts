import { useCallback } from 'react'

import useAccount from './useAccount'
import useRouter from './useRouter'
import { TypesCodeAgentExecutionConfig } from '../api/api'

const PREFIX = 'helix.codeAgentConfig'

/**
 * Remembers the coding agent a user last chose, per organization and project.
 *
 * Scoped to the user as well as the org/project because a shared browser
 * profile would otherwise hand one person's harness choice to the next, and the
 * choice can imply whose subscription authenticates the run.
 *
 * localStorage rather than the server: this is a per-browser convenience, not
 * org policy. The org's allow list stays authoritative — a remembered config is
 * only reapplied if it is still runnable.
 */
export function useRememberedCodeAgentConfig() {
  const router = useRouter()
  const account = useAccount()

  const orgId = router.params.org_id || 'no-org'
  const projectId = router.params.project_id || 'no-project'
  const userId = account.user?.id || 'anonymous'
  const storageKey = `${PREFIX}.${userId}.${orgId}.${projectId}`

  const read = useCallback((): TypesCodeAgentExecutionConfig | undefined => {
    try {
      const raw = window.localStorage.getItem(storageKey)
      if (!raw) return undefined
      const parsed = JSON.parse(raw) as TypesCodeAgentExecutionConfig
      // A stored blob with no runtime or model cannot start anything, so treat
      // it as absent rather than handing a half-config to the caller.
      return parsed?.runtime && parsed?.model ? parsed : undefined
    } catch {
      // Private mode, quota, or a value from an older shape. Losing a
      // convenience default is not worth surfacing an error for.
      return undefined
    }
  }, [storageKey])

  const write = useCallback((config?: TypesCodeAgentExecutionConfig) => {
    try {
      if (!config?.runtime || !config?.model) return
      window.localStorage.setItem(storageKey, JSON.stringify(config))
    } catch {
      // As above — a browser that refuses storage still works, it just forgets.
    }
  }, [storageKey])

  return { read, write }
}

export default useRememberedCodeAgentConfig
