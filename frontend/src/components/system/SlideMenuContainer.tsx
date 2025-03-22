import React, { FC, useState, useRef, useEffect, ReactNode } from 'react'

interface SlideMenuContainerProps {
  children: ReactNode;
  menuType: string; // Identifier for the menu type
}

// Animation duration in ms
const ANIMATION_DURATION = 800;

// Animation states
const POSITION = {
  CENTER: 'translateX(0)',
  LEFT: 'translateX(-100%)',
  RIGHT: 'translateX(100%)'
};

const SlideMenuContainer: FC<SlideMenuContainerProps> = ({ 
  children,
  menuType
}) => {
  // Using separate state for position gives us more control
  const [position, setPosition] = useState(POSITION.CENTER)
  const containerRef = useRef<HTMLDivElement>(null)
  
  // Record what menus are currently visible
  useEffect(() => {
    window._activeMenus = window._activeMenus || {};
    window._activeMenus[menuType] = true;
    
    console.log(`Menu mounted: ${menuType}`);
    
    return () => {
      if (window._activeMenus) {
        delete window._activeMenus[menuType];
      }
      console.log(`Menu unmounted: ${menuType}`);
    };
  }, [menuType]);
  
  // Listen for menu change events - DISABLED due to animation bugs
  /*
  useEffect(() => {
    const handleMenuChange = (e: CustomEvent) => {
      if (!e.detail) return;
      
      const { from, to, direction } = e.detail;
      
      // If this is the menu being navigated away from (exiting)
      if (from === menuType) {
        console.log(`[ANIMATION] Menu "${menuType}" EXITING with direction "${direction}". Current position: ${position}`);
        
        // Slide out in the specified direction
        if (direction === 'left') {
          setPosition(POSITION.LEFT);  // Slide out to the left
          console.log(`[ANIMATION] Position set to LEFT (-100%) for "${menuType}"`);
        } else {
          setPosition(POSITION.RIGHT); // Slide out to the right
          console.log(`[ANIMATION] Position set to RIGHT (100%) for "${menuType}"`);
        }
      }
      
      // If this is the menu being navigated to (entering)
      if (to === menuType) {
        console.log(`[ANIMATION] Menu "${menuType}" ENTERING. Other menu exiting: "${direction}". Current position: ${position}`);
        
        // First position offscreen in the opposite direction of exit
        // If the other menu is exiting left, this one starts from right
        if (direction === 'left') {
          console.log(`[ANIMATION] Setting initial position to RIGHT (100%) for "${menuType}" since other menu is exiting LEFT`);
          setPosition(POSITION.RIGHT); // Start from the right (when other exits left)
        } else {
          console.log(`[ANIMATION] Setting initial position to LEFT (-100%) for "${menuType}" since other menu is exiting RIGHT`);
          setPosition(POSITION.LEFT);  // Start from the left (when other exits right)
        }
        
        // Then after a brief delay, slide to center
        setTimeout(() => {
          console.log(`[ANIMATION] Moving "${menuType}" to CENTER (0). Previous position: ${position}`);
          setPosition(POSITION.CENTER);
        }, 50);
      }
    };
    
    window.addEventListener('menu-change' as any, handleMenuChange);
    
    return () => {
      window.removeEventListener('menu-change' as any, handleMenuChange);
    };
  }, [menuType]);
  */
  
  // Apply transitions with cubic bezier for smooth animation
  const transitionDuration = `${ANIMATION_DURATION/1000}s`;
  
  return (
    <div 
      ref={containerRef}
      style={{
        width: '100%',
        height: '100%',
        transition: `transform ${transitionDuration} cubic-bezier(0.33, 1, 0.68, 1)`,
        transform: position,
        willChange: 'transform',
      }}
    >
      {children}
    </div>
  )
}

// Helper function to clean up animation data
export const cleanupAnimationData = () => {
  localStorage.removeItem('last_resource_type');
  if (window._activeMenus) {
    window._activeMenus = {};
  }
}

// Helper function to trigger menu change animations
export const triggerMenuChange = (
  from: string, 
  to: string, 
  direction: 'left' | 'right' = 'right', 
  isOrgSwitch: boolean = false
) => {
  // DISABLED: Animation system temporarily disabled due to bugs
  console.log(`[ANIMATION DISABLED] Would have animated from "${from}" to "${to}" with direction "${direction}"`);
  
  // Original code commented out:
  /*
  console.log(`[ANIMATION TRIGGER] From "${from}" to "${to}". Direction: "${direction}". isOrgSwitch: ${isOrgSwitch}. What this means:`);
  console.log(`[ANIMATION TRIGGER] - Menu "${from}" will exit by moving ${direction.toUpperCase()}`);
  console.log(`[ANIMATION TRIGGER] - Menu "${to}" will enter from the ${direction === 'left' ? 'RIGHT' : 'LEFT'}`);
  
  // Only trigger if the menus exist or if we're specifically targeting them
  if (window._activeMenus && (window._activeMenus[from] || window._activeMenus[to])) {
    // Create an event with direction info
    const event = new CustomEvent('menu-change', { 
      detail: { 
        from, 
        to, 
        direction,
        isOrgSwitch
      } 
    });
    window.dispatchEvent(event);
    console.log(`[ANIMATION TRIGGER] Event dispatched`);
  } else {
    console.log(`[ANIMATION TRIGGER] Animation NOT triggered - menus not ready. Active menus:`, window._activeMenus);
  }
  */
}

// Simple wrapper for menu containers
export const SlideMenuWrapper: FC<{
  children: ReactNode
}> = ({
  children
}) => {
  return (
    <div style={{
      width: '100%',
      height: '100%',
      overflow: 'hidden',
    }}>
      {children}
    </div>
  )
}

// Set up window object for TypeScript
declare global {
  interface Window {
    _activeMenus?: Record<string, boolean>;
  }
}

// Clean up on load
if (typeof window !== 'undefined') {
  window.addEventListener('load', cleanupAnimationData);
}

export default SlideMenuContainer 