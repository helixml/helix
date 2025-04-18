import React, { createContext, useMemo, useCallback, ReactNode } from 'react'
import { useRoute } from 'react-router5'
import router, { useApplicationRoute } from '../router'

import {
  IRouterNavigateFunction,
} from '../types'

export interface IRouterContext {
  name: string,
  params: Record<string, string>,
  render: () => JSX.Element,
  getTitle?: () => JSX.Element,
  meta: Record<string, any>,
  navigate: IRouterNavigateFunction,
  navigateReplace: IRouterNavigateFunction,
  setParams: {
    (params: Record<string, string>, replace?: boolean): void,
  },
  mergeParams: {
    (params: Record<string, string>): void,
  },
  replaceParams: {
    (params: Record<string, string>): void,
  },
  removeParams: {
    (params: string[]): void,
  },
}

export const RouterContext = createContext<IRouterContext>({
  name: '',
  params: {},
  render: () => <div>Page Not Found</div>,
  meta: {},
  navigate: () => {},
  navigateReplace: () => {},
  setParams: () => {},
  mergeParams: () => {},
  replaceParams: () => {},
  removeParams: () => {},
})

export const useRouterContext = (): IRouterContext => {
  const { route } = useRoute()
  const appRoute = useApplicationRoute()
  const meta = useMemo(() => {
    return appRoute.meta
  }, [
    appRoute,
  ])
  const navigate = useCallback((name: string, params?: Record<string, any>) => {
    params ?
      router.navigate(name, params) :
      router.navigate(name)
  }, [])

  const navigateReplace = useCallback((name: string, params?: Record<string, any>) => {
    params ?
      router.navigate(name, params, { replace: true }) :
      router.navigate(name, {}, { replace: true })
  }, [])

  const setParams = useCallback((params: Record<string, string>, replace = false) => {
    router.navigate(route.name, replace ? params : Object.assign({}, route.params, params))
  }, [
    route.name,
    route.params,
  ])

  const mergeParams = useCallback((params: Record<string, string>) => {
    router.navigate(route.name, Object.assign({}, route.params, params), { replace: true })
  }, [
    route.name,
    route.params,
  ])

  const replaceParams = useCallback((params: Record<string, string>) => {
    router.navigate(route.name, params, { replace: true })
  }, [
    route.name,
    route.params,
  ])

  const removeParams = useCallback((params: string[]) => {
    // reduce the current params and remove the parans list
    const newParams = Object.keys(route.params).reduce((acc: Record<string, string>, key) => {
      if(params.includes(key)) return acc
      acc[key] = route.params[key]
      return acc
    }, {})
    router.navigate(route.name, newParams)
  }, [
    route.name,
    route.params,
  ])

  const render = useCallback(() => {
    return appRoute.render()
  }, [
    appRoute,
  ])

  const contextValue = useMemo<IRouterContext>(() => ({
    name: route.name,
    params: route.params,
    meta,
    navigate,
    navigateReplace,
    setParams,
    mergeParams,
    replaceParams,
    removeParams,
    render,
  }), [
    route.name,
    route.params,
    meta,
    navigate,
    navigateReplace,
    setParams,
    removeParams,
    render,
  ])
  return contextValue
}

export const RouterContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useRouterContext()
  return (
    <RouterContext.Provider value={ value }>
      { children }
    </RouterContext.Provider>
  )
}