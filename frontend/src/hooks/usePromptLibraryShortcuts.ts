/**
 * usePromptLibraryShortcuts - Keyboard shortcuts for the prompt library
 *
 * Shortcuts:
 * - Ctrl/Cmd + Shift + P: Toggle prompt library sidebar
 * - Ctrl/Cmd + Shift + K: Quick search prompts (opens search-focused view)
 * - Escape: Close prompt library
 *
 * Usage:
 * const { isOpen, setIsOpen, searchFocused, setSearchFocused } = usePromptLibraryShortcuts()
 */

import { useState, useEffect, useCallback } from 'react'

interface UsePromptLibraryShortcutsOptions {
  // Optional callback when library is opened
  onOpen?: () => void
  // Optional callback when library is closed
  onClose?: () => void
  // Whether shortcuts are enabled (e.g., disable when typing in input)
  enabled?: boolean
}

interface UsePromptLibraryShortcutsReturn {
  // Whether the prompt library is open
  isOpen: boolean
  setIsOpen: (open: boolean) => void
  // Whether search should be focused on open
  searchFocused: boolean
  setSearchFocused: (focused: boolean) => void
  // Toggle function
  toggle: () => void
  // Open with search focused
  openWithSearch: () => void
}

export function usePromptLibraryShortcuts(
  options: UsePromptLibraryShortcutsOptions = {}
): UsePromptLibraryShortcutsReturn {
  const { onOpen, onClose, enabled = true } = options

  const [isOpen, setIsOpenState] = useState(false)
  const [searchFocused, setSearchFocused] = useState(false)

  const setIsOpen = useCallback((open: boolean) => {
    setIsOpenState(open)
    if (open) {
      onOpen?.()
    } else {
      onClose?.()
      setSearchFocused(false)
    }
  }, [onOpen, onClose])

  const toggle = useCallback(() => {
    setIsOpen(!isOpen)
  }, [isOpen, setIsOpen])

  const openWithSearch = useCallback(() => {
    setSearchFocused(true)
    setIsOpen(true)
  }, [setIsOpen])

  useEffect(() => {
    if (!enabled) return

    const handleKeyDown = (e: KeyboardEvent) => {
      // Check for modifier key (Cmd on Mac, Ctrl on Windows/Linux)
      const isMod = e.metaKey || e.ctrlKey
      const isShift = e.shiftKey

      // Escape to close
      if (e.key === 'Escape' && isOpen) {
        e.preventDefault()
        setIsOpen(false)
        return
      }

      // Ctrl/Cmd + Shift + P: Toggle prompt library
      if (isMod && isShift && e.key.toLowerCase() === 'p') {
        e.preventDefault()
        toggle()
        return
      }

      // Ctrl/Cmd + Shift + K: Open with search focused
      if (isMod && isShift && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        openWithSearch()
        return
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [enabled, isOpen, setIsOpen, toggle, openWithSearch])

  return {
    isOpen,
    setIsOpen,
    searchFocused,
    setSearchFocused,
    toggle,
    openWithSearch,
  }
}

export default usePromptLibraryShortcuts
