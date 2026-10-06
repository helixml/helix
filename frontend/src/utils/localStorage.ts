/**
 * Local storage utility with TTL (Time To Live) support
 */

export const SELECTED_ORG_STORAGE_KEY = 'selected_org'

interface StorageItem<T> {
  value: T
  expiry: number
}

/**
 * Set an item in localStorage with TTL
 * @param key - Storage key
 * @param value - Value to store
 * @param ttlHours - Time to live in hours
 */
export const setWithTTL = <T>(key: string, value: T, ttlHours: number): void => {
  const item: StorageItem<T> = {
    value,
    expiry: Date.now() + (ttlHours * 60 * 60 * 1000)
  }
  localStorage.setItem(key, JSON.stringify(item))
}

/**
 * Get an item from localStorage with TTL
 * @param key - Storage key
 * @returns The stored value or null if expired/not found
 */
export const getWithTTL = <T>(key: string): T | null => {
  try {
    const itemStr = localStorage.getItem(key)
    if (!itemStr) return null

    const item: StorageItem<T> = JSON.parse(itemStr)
    
    // Check if expired
    if (Date.now() > item.expiry) {
      localStorage.removeItem(key)
      return null
    }

    return item.value
  } catch (error) {
    // If parsing fails, remove the corrupted item
    localStorage.removeItem(key)
    return null
  }
}

/**
 * Remove an item from localStorage
 * @param key - Storage key
 */
export const removeWithTTL = (key: string): void => {
  localStorage.removeItem(key)
}

/**
 * Check if an item exists and is not expired
 * @param key - Storage key
 * @returns True if item exists and is not expired
 */
export const hasValidTTL = (key: string): boolean => {
  return getWithTTL(key) !== null
}

/**
 * The selected org is remembered per browser, not per user. Reading and
 * writing it through these helpers keeps that single source of truth in one
 * place — in particular so logout can clear it. Leaving a stale slug behind
 * meant the next user to log in on the same browser was navigated straight
 * into an org they have no membership in, producing a wall of 403s and a
 * dead org switcher.
 */
export const getSelectedOrg = (): string | undefined => {
  try {
    return localStorage.getItem(SELECTED_ORG_STORAGE_KEY) || undefined
  } catch {
    return undefined
  }
}

export const setSelectedOrg = (orgSlug: string): void => {
  try {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, orgSlug)
  } catch {
    // Storage can be unavailable (private mode, quota). Remembering the org is
    // a convenience, never a correctness requirement.
  }
}

export const clearSelectedOrg = (): void => {
  try {
    localStorage.removeItem(SELECTED_ORG_STORAGE_KEY)
  } catch {
    // See setSelectedOrg.
  }
}
