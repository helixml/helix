import React, { createContext, useState, useCallback, ReactNode } from 'react'

export type ISnackbarSeverity = 'error' | 'warning' | 'info' | 'success'

export interface ISnackbarData {
  id: string,
  message: string,
  severity: ISnackbarSeverity,
}

export interface ISnackbarContext {
  snackbars: ISnackbarData[],
  setSnackbar: {
    (message: string, severity?: ISnackbarSeverity): void,
  },
  dismissSnackbar: {
    (id: string): void,
  },
}

export const SnackbarContext = createContext<ISnackbarContext>({
  snackbars: [],
  setSnackbar: () => {},
  dismissSnackbar: () => {},
})

let snackbarSequence = 0

export const useSnackbarContext = (): ISnackbarContext => {
  const [snackbars, setSnackbars] = useState<ISnackbarData[]>([])

  const setSnackbar = useCallback((message: string, severity?: ISnackbarSeverity) => {
    if (!message) {
      setSnackbars([])
    } else {
      snackbarSequence += 1
      setSnackbars((current) => [
        ...current,
        {
          id: `notification-${Date.now()}-${snackbarSequence}`,
          message,
          severity: severity || 'info',
        },
      ].slice(-8))
    }
  }, [])

  const dismissSnackbar = useCallback((id: string) => {
    setSnackbars((current) => current.filter((snackbar) => snackbar.id !== id))
  }, [])
  
  return {
    snackbars,
    setSnackbar,
    dismissSnackbar,
  }
}

export const SnackbarContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useSnackbarContext()
  return (
    <SnackbarContext.Provider value={ value }>
      { children }
    </SnackbarContext.Provider>
  )
}
