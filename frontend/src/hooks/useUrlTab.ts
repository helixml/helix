import { useCallback, useEffect, useState } from 'react'

const readUrlTab = (param: string): string | null => {
  if (typeof window === 'undefined') return null
  return new URLSearchParams(window.location.search).get(param)
}

const writeUrlTab = (param: string, value: string) => {
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  url.searchParams.set(param, value)
  window.history.replaceState({}, '', url.toString())
}

// useUrlTab persists the active tab in the URL query so a page refresh or
// shared link lands on the same tab. `validValues` constrains which strings
// are accepted — anything else falls back to `defaultTab`.
export function useUrlTab<T extends string>(
  paramName: string,
  validValues: readonly T[],
  defaultTab: T,
): [T, (next: T) => void] {
  const isValid = useCallback((v: string | null): v is T => !!v && (validValues as readonly string[]).includes(v), [validValues])

  const [tab, setTabState] = useState<T>(() => {
    const fromUrl = readUrlTab(paramName)
    return isValid(fromUrl) ? fromUrl : defaultTab
  })

  useEffect(() => {
    writeUrlTab(paramName, tab)
  }, [tab, paramName])

  const setTab = useCallback((next: T) => setTabState(next), [])

  return [tab, setTab]
}

export default useUrlTab
