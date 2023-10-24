import { useContext, useCallback } from 'react'

import {
  SnackbarContext,
} from '../contexts/snackbar'

export const extractErrorMessage = (error: any): string => {
  if(error.response && error.response.data) {
    if (error.response.data.message || error.response.data.error) {
      return (error.response.data.message || error.response.data.error) as string
    }
    if (error.response.data) return error.response.data as string
    return error.toString()
  }
  else {
    return error.toString()
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