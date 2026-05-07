import { useState, useEffect, useCallback } from 'react'

/**
 * Hook to track the height of the UserOrgSelector floating menu
 * This is used to dynamically adjust sidebar content height so it doesn't 
 * get covered by the variable-height floating user menu
 */
export const useUserMenuHeight = () => {
  const [userMenuHeight, setUserMenuHeight] = useState(0)

  const updateHeight = useCallback(() => {
    // The floating user menu (Admin Panel / Account Settings / Connected Services
    // + user profile) is rendered by UserOrgSelector. In the mounted-in-sidebar
    // case it's position: absolute, in the Portal/compact case it's position:
    // fixed — both pin to bottom: 0; left: 0. Match either.
    const floatingMenus = document.querySelectorAll('[data-compact-user-menu]')
    let totalHeight = 0

    for (const menu of floatingMenus) {
      if (!(menu instanceof HTMLElement)) continue
      let parent: HTMLElement | null = menu.parentElement
      while (parent) {
        const cs = window.getComputedStyle(parent)
        if ((cs.position === 'absolute' || cs.position === 'fixed') &&
            cs.bottom === '0px' &&
            cs.left === '0px') {
          // Visible = opacity 1 AND interactive. Hidden states have opacity 0 + pointerEvents none.
          if (cs.opacity === '1' && cs.pointerEvents === 'auto') {
            totalHeight = Math.max(totalHeight, parent.offsetHeight)
          }
          break
        }
        parent = parent.parentElement
      }
    }

    setUserMenuHeight(totalHeight)
  }, [])

  useEffect(() => {
    // Initial height calculation
    updateHeight()

    // Set up a ResizeObserver to watch for changes in the floating menu size
    const observer = new ResizeObserver(() => {
      updateHeight()
    })

    // Set up a MutationObserver to watch for DOM changes (menu expanding/collapsing)
    const mutationObserver = new MutationObserver(() => {
      updateHeight()
    })

    // Observe changes to the body (where the floating menu is attached)
    const targetNode = document.body
    if (targetNode) {
      mutationObserver.observe(targetNode, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: ['style', 'class']
      })
    }

    // Also set up a periodic check as a fallback
    const intervalId = setInterval(updateHeight, 1000)

    return () => {
      observer.disconnect()
      mutationObserver.disconnect()
      clearInterval(intervalId)
    }
  }, [updateHeight])

  return userMenuHeight
}

export default useUserMenuHeight 