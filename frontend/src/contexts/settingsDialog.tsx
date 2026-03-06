import React, { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react'

export type SettingsDialogName = 'admin' | 'connected-services' | 'account'

export interface SettingsDialogOptions {
  tab?: string
}

interface SettingsDialogContextType {
  activeDialog: SettingsDialogName | null
  dialogOptions: SettingsDialogOptions
  openDialog: (name: SettingsDialogName, options?: SettingsDialogOptions) => void
  closeDialog: () => void
}

const DIALOG_PARAM = 'dialog'
const DIALOG_TAB_PARAM = 'dialog_tab'

function getDialogFromURL(): { name: SettingsDialogName | null; options: SettingsDialogOptions } {
  const params = new URLSearchParams(window.location.search)
  const name = params.get(DIALOG_PARAM) as SettingsDialogName | null
  const tab = params.get(DIALOG_TAB_PARAM) || undefined
  if (name && ['admin', 'connected-services', 'account'].includes(name)) {
    return { name, options: tab ? { tab } : {} }
  }
  return { name: null, options: {} }
}

function setDialogInURL(name: SettingsDialogName | null, options: SettingsDialogOptions = {}) {
  const url = new URL(window.location.href)
  if (name) {
    url.searchParams.set(DIALOG_PARAM, name)
    if (options.tab) {
      url.searchParams.set(DIALOG_TAB_PARAM, options.tab)
    } else {
      url.searchParams.delete(DIALOG_TAB_PARAM)
    }
  } else {
    url.searchParams.delete(DIALOG_PARAM)
    url.searchParams.delete(DIALOG_TAB_PARAM)
  }
  window.history.replaceState({}, '', url.toString())
}

const SettingsDialogContext = createContext<SettingsDialogContextType>({
  activeDialog: null,
  dialogOptions: {},
  openDialog: () => {},
  closeDialog: () => {},
})

export const useSettingsDialog = () => useContext(SettingsDialogContext)

export const SettingsDialogProvider = ({ children }: { children: ReactNode }) => {
  const initial = getDialogFromURL()
  const [activeDialog, setActiveDialog] = useState<SettingsDialogName | null>(initial.name)
  const [dialogOptions, setDialogOptions] = useState<SettingsDialogOptions>(initial.options)

  const openDialog = useCallback((name: SettingsDialogName, options: SettingsDialogOptions = {}) => {
    setDialogOptions(options)
    setActiveDialog(name)
    setDialogInURL(name, options)
  }, [])

  const closeDialog = useCallback(() => {
    setActiveDialog(null)
    setDialogOptions({})
    setDialogInURL(null)
  }, [])

  // Handle browser back/forward
  useEffect(() => {
    const handlePopState = () => {
      const { name, options } = getDialogFromURL()
      setActiveDialog(name)
      setDialogOptions(options)
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  return (
    <SettingsDialogContext.Provider value={{ activeDialog, dialogOptions, openDialog, closeDialog }}>
      {children}
    </SettingsDialogContext.Provider>
  )
}
