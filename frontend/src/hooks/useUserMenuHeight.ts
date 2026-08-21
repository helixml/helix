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
// Now: find the bottom-pinned overlay once, then ResizeObserver it directly.
// ResizeObserver only fires when that element actually changes size, and Chrome
// schedules its callbacks at a layout-safe point in the frame.
//
// The overlay is found by its `data-user-menu-overlay` marker rather than by
// walking up from the avatar looking for the first `position: fixed/absolute`
// ancestor pinned to `bottom: 0`. That walk had no way to tell the menu from
// anything else it happened to pass through: on a phone the nav Drawer is a
// temporary one, whose Paper is `position: fixed; bottom: 0`, so the walk
// stopped there and reported the height of the whole drawer. Sidebar then
// computed `calc(100% - 844px)` and clipped the entire chat list to zero
// height. Desktop escaped it only because Layout gives the permanent drawer
// `position: relative`.
export const useUserMenuHeight = () => {
  const [userMenuHeight, setUserMenuHeight] = useState(0)

  useEffect(() => {
    let cancelled = false
    let ro: ResizeObserver | null = null
    let retryTimer: ReturnType<typeof setTimeout> | null = null

    const tryAttach = () => {
      if (cancelled) return

      const overlay = document.querySelector('[data-user-menu-overlay]')
      if (!(overlay instanceof HTMLElement)) {
        // Component hasn't mounted yet (or has unmounted). Retry slowly.
        // 1s is fine for a layout-sizing decision; no-one notices a 1s wait
        // for the sidebar bottom padding to settle.
        retryTimer = setTimeout(tryAttach, 1000)
        return
      }

      // In compact mode the menu is portalled into a wrapper that carries the
      // show/hide state, so visibility is read from the overlay and the couple
      // of nodes above it rather than the overlay alone.
      const isHidden = (el: HTMLElement | null, depth: number): boolean => {
        for (let i = 0; el && i <= depth; i += 1) {
          const cs = window.getComputedStyle(el)
          if (cs.opacity !== '1' || cs.pointerEvents === 'none') return true
          el = el.parentElement
        }
        return false
      }

      const measure = () => {
        setUserMenuHeight(isHidden(overlay, 2) ? 0 : overlay.offsetHeight)
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
