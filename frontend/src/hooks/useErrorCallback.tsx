import { useContext, useCallback } from 'react'

import {
  SnackbarContext,
} from '../contexts/snackbar'

export const extractErrorMessage = (error: any): string => {
  if(error.response && error.response.data) {
    const data = error.response.data
    if (typeof data.message === 'string' && data.message) return data.message
    if (typeof data.error === 'string' && data.error) return data.error
    return typeof data === 'string' ? data : JSON.stringify(data)
  }
  else if(error.error) {
    return error.error
  }
  else if(error.message) {
    return error.message
  }
  else {
    return JSON.stringify(error)
  }
}

export function useErrorCallback<T = void>(handler: {
  (): Promise<T | void>,
}, snackbarActive = true) {
  const snackbar = useContext(SnackbarContext)
  const callback = useCallback(async () => {
    try {
      const result = await handler()
      return result
    } catch(e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      if(snackbarActive !== false) snackbar.setSnackbar(errorMessage, 'error')
    }
    return
  }, [
    handler,
    snackbarActive,
  ])
  return callback
}

export default useErrorCallback
