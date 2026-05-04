import { useCallback, useEffect, useState } from 'react'
import useRouter from './useRouter'

const readUrlTab = (param: string): string | null => {
  if (typeof window === 'undefined') return null
  return new URLSearchParams(window.location.search).get(param)
}

// useUrlTab persists the active tab in the URL query so a page refresh or
// shared link lands on the same tab. `validValues` constrains which strings
// are accepted — anything else falls back to `defaultTab`.
//
// We route URL writes through router5's `mergeParams` (replace), not raw
// window.history.replaceState. Bypassing router5 corrupts its back-stack —
// symptom was that Browser Back from a sandbox detail page skipped the
// sandbox list and went to whatever was visited before it (e.g. QA), because
// the raw replaceState left router5's transition state out of sync with the
// real browser URL, so popstate handling went through the wrong entry.
export function useUrlTab<T extends string>(
  paramName: string,
  validValues: readonly T[],
  defaultTab: T,
): [T, (next: T) => void] {
  const router = useRouter()
  const isValid = useCallback((v: string | null): v is T => !!v && (validValues as readonly string[]).includes(v), [validValues])

  const [tab, setTabState] = useState<T>(() => {
    const fromUrl = readUrlTab(paramName)
    return isValid(fromUrl) ? fromUrl : defaultTab
  })

  useEffect(() => {
    if (router.params?.[paramName] !== tab) {
      router.mergeParams({ [paramName]: tab })
    }
  }, [tab, paramName, router])

  const setTab = useCallback((next: T) => setTabState(next), [])

  return [tab, setTab]
}

export default useUrlTab
