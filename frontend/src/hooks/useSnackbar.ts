import { useContext, useCallback } from 'react'

import {
  SnackbarContext,
} from '../contexts/snackbar'

export const useSnackbar = () => {
  const snackbar = useContext(SnackbarContext)

  const error = useCallback((message: string) => {
    snackbar.setSnackbar(message, 'error')
  }, [])

  const info = useCallback((message: string) => {
    snackbar.setSnackbar(message, 'info')
  }, [])

  const success = useCallback((message: string) => {
    snackbar.setSnackbar(message, 'success')
  }, [])

  const warning = useCallback((message: string) => {
    snackbar.setSnackbar(message, 'warning')
  }, [])

  return {
    error,
    info,
    success,
    warning,
  }
}

export default useSnackbar