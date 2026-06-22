import { useState, useCallback, useEffect, useRef } from 'react'

const STORAGE_KEY = 'helix_browser_notif_disabled'
const SHOWN_KEY = 'helix_browser_notif_shown'

function loadShownIds(): Set<string> {
  try {
    const raw = sessionStorage.getItem(SHOWN_KEY)
    if (raw) return new Set(JSON.parse(raw) as string[])
  } catch {
    // ignore
  }
  return new Set()
}

function saveShownId(id: string): void {
  try {
    const raw = sessionStorage.getItem(SHOWN_KEY)
    const ids: string[] = raw ? JSON.parse(raw) : []
    if (!ids.includes(id)) {
      ids.push(id)
      sessionStorage.setItem(SHOWN_KEY, JSON.stringify(ids))
    }
  } catch {
    // ignore
  }
}

type PermissionState = 'default' | 'granted' | 'denied' | 'unsupported'

function getPermissionState(): PermissionState {
  if (typeof window === 'undefined' || !('Notification' in window)) {
    return 'unsupported'
  }
  return Notification.permission as PermissionState
}

function isDisabledByUser(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

export function useBrowserNotifications() {
  const [permission, setPermission] = useState<PermissionState>(getPermissionState)
  const [disabledByUser, setDisabledByUser] = useState(isDisabledByUser)
  // Track which event IDs we've already shown a notification for, so we don't
  // fire duplicates across re-renders or remounts.
  const shownRef = useRef<Set<string>>(loadShownIds())

  // Re-sync permission state when the tab regains focus (user may have changed
  // it in browser settings while we were in the background).
  useEffect(() => {
    const handler = () => setPermission(getPermissionState())
    window.addEventListener('focus', handler)
    return () => window.removeEventListener('focus', handler)
  }, [])

  const requestPermission = useCallback(async () => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return
    }
    try {
      const result = await Notification.requestPermission()
      setPermission(result as PermissionState)
      if (result === 'granted') {
        setDisabledByUser(false)
        localStorage.removeItem(STORAGE_KEY)
      }
    } catch {
      // Safari may throw on the promise-based API
    }
  }, [])

  const setOptOut = useCallback((disabled: boolean) => {
    setDisabledByUser(disabled)
    try {
      if (disabled) {
        localStorage.setItem(STORAGE_KEY, 'true')
      } else {
        localStorage.removeItem(STORAGE_KEY)
      }
    } catch {
      // localStorage may be unavailable
    }
  }, [])

  const fireNotification = useCallback(
    (
      id: string,
      title: string,
      body: string,
      onClick?: () => void,
    ) => {
      if (permission !== 'granted' || disabledByUser) return
      if (shownRef.current.has(id)) return
      shownRef.current.add(id)
      saveShownId(id)

      try {
        const notification = new Notification(title, {
          body,
          icon: '/favicon.ico',
          tag: id, // browser deduplicates by tag
        })

        notification.onclick = () => {
          window.focus()
          notification.close()
          onClick?.()
        }
      } catch {
        // Notification constructor can fail in some environments
      }
    },
    [permission, disabledByUser],
  )

  // Whether the user should be prompted to enable notifications.
  // True when the browser supports them, the user hasn't denied or opted out,
  // and we haven't been granted permission yet.
  const shouldPrompt =
    permission === 'default' && !disabledByUser

  // Whether notifications are fully active.
  const isEnabled = permission === 'granted' && !disabledByUser

  return {
    permission,
    isEnabled,
    shouldPrompt,
    disabledByUser,
    requestPermission,
    setOptOut,
    fireNotification,
  }
}