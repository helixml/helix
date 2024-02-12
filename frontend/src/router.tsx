import createRouter, { Route } from 'router5'
import { useRoute } from 'react-router5'
import browserPlugin from 'router5-plugin-browser'
import Box from '@mui/material/Box'

import Session from './pages/Session'
import Account from './pages/Account'
import Tools from './pages/Tools'
import Tool from './pages/Tool'
import Dashboard from './pages/Dashboard'
import New from './pages/New'

import SessionBadgeKey from './components/session/SessionBadgeKey'
import SessionTitle from './components/session/SessionTitle'

import { FilestoreContextProvider } from './contexts/filestore'
import Files from './pages/Files'

// extend the base router5 route to add metadata and self rendering
export interface IApplicationRoute extends Route {
  render: () => JSX.Element,
  getTitle?: () => JSX.Element,
  meta: Record<string, any>,
}

export const NOT_FOUND_ROUTE: IApplicationRoute = {
  name: 'notfound',
  path: '/notfound',
  meta: {
    title: 'Page Not Found',
  },
  render: () => <div>Page Not Found</div>,
}

const routes: IApplicationRoute[] = [{
  name: 'new',
  path: '/',
  meta: {
    title: 'New Session',
    sidebar: true,
  },
  render: () => (
      <New />
  ),
}, {
  name: 'files',
  path: '/files',
  meta: {
    title: 'Files',
    sidebar: true,
  },
  render: () => (
    <FilestoreContextProvider>
      <Files />
    </FilestoreContextProvider>
  ),
}, {
  name: 'tools',
  path: '/tools',
  meta: {
    title: 'Tools',
    sidebar: true,
  },
  render: () => (
    <Tools />
  ),
}, {
  name: 'tool',
  path: '/tool/:tool_id',
  meta: {
    title: 'Tools : Edit',
    sidebar: false,
  },
  render: () => (
    <Tool />
  ),
}, {
  name: 'session',
  path: '/session/:session_id',
  meta: {
    title: 'Session',
    sidebar: true,
  },
  getTitle: () => {
    return (
      <SessionTitle />
    )
  },
  render: () => (
      <Session />
  ),
}, {
  name: 'dashboard',
  path: '/dashboard',
  meta: {
    title: 'Dashboard',
    sidebar: true,
    background: '#ffffff'
  },
  render: () => (
      <Dashboard />
  ),
}, {
  name: 'account',
  path: '/account',
  meta: {
    title: 'Account',
    sidebar: true,
  },
  render: () => <Account />,
}, NOT_FOUND_ROUTE]

export const router = createRouter(routes, {
  defaultRoute: 'notfound',
  queryParamsMode: 'loose',
})

router.usePlugin(browserPlugin())
router.start()

export function useApplicationRoute(): IApplicationRoute {
  const { route } = useRoute()
  const fullRoute = routes.find(r => r.name == route?.name) || NOT_FOUND_ROUTE
  return fullRoute
}

export function RenderPage() {
  const route = useApplicationRoute()
  return route.render()
}

export default router