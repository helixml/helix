import { useCallback, useEffect, useState } from 'react'
import useRouter from './useRouter'
import { ViewMode } from '../components/widgets/ViewModeToggle'

const isViewMode = (v: string | null): v is ViewMode => v === 'table' || v === 'cards'

const readUrlMode = (param: string): ViewMode | null => {
  if (typeof window === 'undefined') return null
  const v = new URLSearchParams(window.location.search).get(param)
  return isViewMode(v) ? v : null
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
//
// Important: we route URL writes through router5's `mergeParams` (replace),
// not raw window.history.replaceState. Bypassing router5 corrupts its
// internal back-stack — symptom was that Browser Back from a sandbox detail
// page jumped past the sandbox list straight to the previous page (e.g. QA),
// because router5's transition state went out of sync with the browser URL.
export function useViewMode(storageKey: string, defaultMode: ViewMode = 'table', urlParam = 'view'): [ViewMode, (mode: ViewMode) => void] {
  const router = useRouter()
  const [mode, setModeState] = useState<ViewMode>(() => readUrlMode(urlParam) ?? readStorageMode(storageKey) ?? defaultMode)

  useEffect(() => {
    if (router.params?.[urlParam] !== mode) {
      router.mergeParams({ [urlParam]: mode })
    }
    writeStorageMode(storageKey, mode)
  }, [mode, storageKey, urlParam, router])

  const setMode = useCallback((next: ViewMode) => setModeState(next), [])

  return [mode, setMode]
}

export default useViewMode
