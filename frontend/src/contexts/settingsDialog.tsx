import React, { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react'
import { router } from '../router'

export type SettingsDialogName = 'admin' | 'connected-services' | 'account' | 'project-settings'

export interface SettingsDialogOptions {
  tab?: string
  projectId?: string
}

interface SettingsDialogContextType {
  activeDialog: SettingsDialogName | null
  dialogOptions: SettingsDialogOptions
  openDialog: (name: SettingsDialogName, options?: SettingsDialogOptions) => void
  closeDialog: () => void
}

const DIALOG_PARAM = 'dialog'
const DIALOG_TAB_PARAM = 'dialog_tab'
const DIALOG_PROJECT_ID_PARAM = 'dialog_project_id'

const VALID_DIALOGS: SettingsDialogName[] = ['admin', 'connected-services', 'account', 'project-settings']

function getDialogFromURL(): { name: SettingsDialogName | null; options: SettingsDialogOptions } {
  const params = new URLSearchParams(window.location.search)
  const name = params.get(DIALOG_PARAM) as SettingsDialogName | null
  const tab = params.get(DIALOG_TAB_PARAM) || undefined
  const projectId = params.get(DIALOG_PROJECT_ID_PARAM) || undefined
  if (name && VALID_DIALOGS.includes(name)) {
    return { name, options: { tab, projectId } }
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
    if (options.projectId) {
      url.searchParams.set(DIALOG_PROJECT_ID_PARAM, options.projectId)
    } else {
      url.searchParams.delete(DIALOG_PROJECT_ID_PARAM)
    }
  } else {
    url.searchParams.delete(DIALOG_PARAM)
    url.searchParams.delete(DIALOG_TAB_PARAM)
    url.searchParams.delete(DIALOG_PROJECT_ID_PARAM)
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

  // Close dialog when the route changes — e.g. navigating to an agent page
  useEffect(() => {
    const subscription = router.subscribe(({ route, previousRoute }) => {
      if (previousRoute && route.name !== previousRoute.name) {
        setActiveDialog(null)
        setDialogOptions({})
      }
    }) as { unsubscribe: () => void } | (() => void)
    return () => {
      if (typeof subscription === 'function') {
        subscription()
      } else {
        subscription.unsubscribe()
      }
    }
  }, [])

  return (
    <SettingsDialogContext.Provider value={{ activeDialog, dialogOptions, openDialog, closeDialog }}>
      {children}
    </SettingsDialogContext.Provider>
  )
}
