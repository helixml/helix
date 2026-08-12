import { useState, useEffect } from 'react'

// Hook to track the height of the UserOrgSelector floating menu, so the
// sidebar's content area can leave room for it (Sidebar.tsx uses
// `calc(100% - ${userMenuHeight}px)`).
//
// Earlier this hook ran a MutationObserver on document.body filtered by
// childList/subtree/attributes(style,class) plus a 1s setInterval, and on every
// fire walked parentElement chains calling getComputedStyle + offsetHeight.
// During streaming (e.g. a spec task chat receiving Zed entry_patches at
// ~60 Hz), every React render flips MUI inline styles somewhere on the page,
// firing the observer constantly and producing forced synchronous layouts on
// the main thread. On Safari this is enough to freeze the tab.
//
// Now: find the bottom-pinned overlay parent once, then ResizeObserver it
// directly. ResizeObserver only fires when that element actually changes size,
// and Chrome schedules its callbacks at a layout-safe point in the frame.
export const useUserMenuHeight = () => {
  const [userMenuHeight, setUserMenuHeight] = useState(0)

  useEffect(() => {
    let cancelled = false
    let ro: ResizeObserver | null = null
    let retryTimer: ReturnType<typeof setTimeout> | null = null

    const findOverlayFor = (menu: HTMLElement): HTMLElement | null => {
      let el: HTMLElement | null = menu
      while (el) {
        const cs = window.getComputedStyle(el)
        if (
          (cs.position === 'absolute' || cs.position === 'fixed') &&
          cs.bottom === '0px'
        ) {
          return el
        }
        el = el.parentElement
      }
      return null
    }

    const tryAttach = () => {
      if (cancelled) return

      const menu = document.querySelector('[data-compact-user-menu]')
      if (!(menu instanceof HTMLElement)) {
        // Component hasn't mounted yet (or has unmounted). Retry slowly.
        // 1s is fine for a layout-sizing decision; no-one notices a 1s wait
        // for the sidebar bottom padding to settle.
        retryTimer = setTimeout(tryAttach, 1000)
        return
      }

      const overlay = findOverlayFor(menu)
      if (!overlay) {
        setUserMenuHeight(0)
        return
      }

      const measure = () => {
        const cs = window.getComputedStyle(overlay)
        const visible =
          cs.opacity === '1' && cs.pointerEvents === 'auto'
        setUserMenuHeight(visible ? overlay.offsetHeight : 0)
      }

      measure()
      ro = new ResizeObserver(measure)
      ro.observe(overlay)
    }

    tryAttach()

    return () => {
      cancelled = true
      ro?.disconnect()
      if (retryTimer) clearTimeout(retryTimer)
    }
  }, [])

  return userMenuHeight
}

export default useUserMenuHeight
