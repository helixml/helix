import React, { useCallback } from 'react'
const win = window as any
export const useTracking = () => {
  const emitEvent = useCallback((ev: any) => {
    if(!win.emitEvent) return
    win.emitEvent(ev)
  }, [])
  
  return {
    emitEvent,
  }
}

export default useTracking