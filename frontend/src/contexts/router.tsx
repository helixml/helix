import React, { FC, createContext, useMemo, useCallback } from 'react'
import { useRoute } from 'react-router5'
import router, { useApplicationRoute } from '../router'

export interface IRouterContext {
  name: string,
  params: Record<string, string>,
  render: () => JSX.Element,
  getTitle?: () => JSX.Element,
  meta: Record<string, any>,
  navigate: {
    (name: string, params?: Record<string, any>): void,
  },
  setParams: {
    (params: Record<string, string>, replace?: boolean): void,
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
  setParams: () => {},
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
  const getTitle = useMemo(() => {
    return appRoute.getTitle
  }, [
    appRoute,
  ])
  const navigate = useCallback((name: string, params?: Record<string, any>) => {
    params ?
      router.navigate(name, params) :
      router.navigate(name)
  }, [])

  const setParams = useCallback((params: Record<string, string>, replace = false) => {
    router.navigate(route.name, replace ? params : Object.assign({}, route.params, params))
  }, [
    route,
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
    route,
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
    getTitle,
    navigate,
    setParams,
    removeParams,
    render,
  }), [
    route.name,
    route.params,
    meta,
    getTitle,
    navigate,
    setParams,
    removeParams,
    render,
  ])
  return contextValue
}

export const RouterContextProvider: FC = ({ children }) => {
  const value = useRouterContext()
  return (
    <RouterContext.Provider value={ value }>
      { children }
    </RouterContext.Provider>
  )
}