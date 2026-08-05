import React, { createContext, useCallback, ReactNode } from 'react'
import { useRoute } from 'react-router5'

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

interface ApplicationRoute {
  render: () => JSX.Element,
  meta: Record<string, any>,
}

// Keep this module independent of ../router. The application router imports every
// page, and pages import this context through useRouter; closing that cycle makes
// Vite refresh only the edited page instead of the entire application.

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

const useRouterContext = (appRoute: ApplicationRoute): IRouterContext => {
  const { route, router } = useRoute()
  const routeParamsKey = JSON.stringify(route.params)
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
    routeParamsKey,
  ])

  const mergeParams = useCallback((params: Record<string, string>) => {
    router.navigate(route.name, Object.assign({}, route.params, params), { replace: true })
  }, [
    route.name,
    routeParamsKey,
  ])

  const replaceParams = useCallback((params: Record<string, string>) => {
    router.navigate(route.name, params, { replace: true })
  }, [
    route.name,
    routeParamsKey,
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
    routeParamsKey,
  ])

  return {
    name: route.name,
    params: route.params,
    meta: appRoute.meta,
    navigate,
    navigateReplace,
    setParams,
    mergeParams,
    replaceParams,
    removeParams,
    render: appRoute.render,
  }
}

export const RouterContextProvider = ({
  appRoute,
  children,
}: {
  appRoute: ApplicationRoute,
  children: ReactNode,
}) => {
  const value = useRouterContext(appRoute)
  return (
    <RouterContext.Provider value={ value }>
      { children }
    </RouterContext.Provider>
  )
}
