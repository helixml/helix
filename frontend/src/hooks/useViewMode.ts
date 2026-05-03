import { useCallback, useEffect, useState } from 'react'
import { ViewMode } from '../components/widgets/ViewModeToggle'

const isViewMode = (v: string | null): v is ViewMode => v === 'table' || v === 'cards'

const readUrlMode = (param: string): ViewMode | null => {
  if (typeof window === 'undefined') return null
  const v = new URLSearchParams(window.location.search).get(param)
  return isViewMode(v) ? v : null
}

const writeUrlMode = (param: string, mode: ViewMode) => {
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  url.searchParams.set(param, mode)
  window.history.replaceState({}, '', url.toString())
}

const readStorageMode = (key: string): ViewMode | null => {
  try {
    const v = window.localStorage.getItem(key)
    return isViewMode(v) ? v : null
  } catch {
    return null
  }
}

const writeStorageMode = (key: string, mode: ViewMode) => {
  try {
    window.localStorage.setItem(key, mode)
  } catch {
    // ignore storage errors
  }
}

// useViewMode persists the table/cards choice via URL query param (so it
// survives shared links) and falls back to localStorage on first load.
export function useViewMode(storageKey: string, defaultMode: ViewMode = 'table', urlParam = 'view'): [ViewMode, (mode: ViewMode) => void] {
  const [mode, setModeState] = useState<ViewMode>(() => readUrlMode(urlParam) ?? readStorageMode(storageKey) ?? defaultMode)

  useEffect(() => {
    writeUrlMode(urlParam, mode)
    writeStorageMode(storageKey, mode)
  }, [mode, storageKey, urlParam])

  const setMode = useCallback((next: ViewMode) => setModeState(next), [])

  return [mode, setMode]
}

export default useViewMode
