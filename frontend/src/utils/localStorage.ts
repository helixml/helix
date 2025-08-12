/**
 * Local storage utility with TTL (Time To Live) support
 */

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
