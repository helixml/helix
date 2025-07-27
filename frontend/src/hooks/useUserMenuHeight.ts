import { useState, useEffect, useCallback } from 'react'

/**
 * Hook to track the height of the UserOrgSelector floating menu
 * This is used to dynamically adjust sidebar content height so it doesn't 
 * get covered by the variable-height floating user menu
 */
export const useUserMenuHeight = () => {
  const [userMenuHeight, setUserMenuHeight] = useState(0)

  const updateHeight = useCallback(() => {
    // Find the floating menu - look for the fixed positioned container that contains user menu
    // This is more specific to the UserOrgSelector implementation
    const floatingMenus = document.querySelectorAll('[data-compact-user-menu]')
    let totalHeight = 0
    
    // Find the parent container that's position: fixed
    for (const menu of floatingMenus) {
      if (menu instanceof HTMLElement) {
        let parent = menu.parentElement
        while (parent) {
          const computedStyle = window.getComputedStyle(parent)
          if (computedStyle.position === 'fixed' && 
              computedStyle.bottom === '0px' && 
              computedStyle.left === '0px') {
            // Check if this floating menu is visible
            const menuComputedStyle = window.getComputedStyle(parent)
            const isVisible = menuComputedStyle.opacity === '1' && menuComputedStyle.pointerEvents === 'auto'
            
                         if (isVisible) {
               totalHeight = Math.max(totalHeight, parent.offsetHeight)
             }
            break
          }
          parent = parent.parentElement
        }
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