import React, { useContext } from 'react'
import {
  AppsContext,
  IAppsContext,
} from '../contexts/apps'

// Simple hook that returns the apps context value
export const useApps = (): IAppsContext => {
  const apps = useContext(AppsContext)
  return apps
}

export default useApps