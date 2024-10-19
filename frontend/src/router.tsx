import createRouter, { Route } from 'router5'
import { useRoute } from 'react-router5'
import browserPlugin from 'router5-plugin-browser'

import Session from './pages/Session'
import Account from './pages/Account'
import Tools from './pages/Tools'
import Tool from './pages/Tool'
import Apps from './pages/Apps'
import App from './pages/App'
import Dashboard from './pages/Dashboard'
import Create from './pages/Create'
import Home from './pages/Home'
import AppStore from './pages/AppStore'
import OpenAPI from './pages/OpenAPI'
import { FilestoreContextProvider } from './contexts/filestore'
import Files from './pages/Files'

// extend the base router5 route to add metadata and self rendering
export interface IApplicationRoute extends Route {
  render: () => JSX.Element,
  meta: Record<string, any>,
}

export const NOT_FOUND_ROUTE: IApplicationRoute = {
  name: 'notfound',
  path: '/notfound',
  meta: {},
  render: () => <div>Page Not Found</div>,
}

const routes: IApplicationRoute[] = [{
  name: 'home',
  path: '/',
  meta: {
    title: 'Home',
    drawer: true,
  },
  render: () => (
      <Home />
  ),
}, {
  name: 'appstore',
  path: '/appstore',
  meta: {
    title: 'App Store',
    drawer: true,
  },
  render: () => (
      <AppStore />
  ),
}, {
  name: 'new',
  path: '/new',
  meta: {
    title: 'New Session',
    drawer: true,
  },
  render: () => (
      <Create />
  ),
}, {
  name: 'files',
  path: '/files',
  meta: {
    drawer: true,
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
    drawer: true,
  },
  render: () => (
    <Tools />
  ),
}, {
  name: 'apps',
  path: '/apps',
  meta: {
    drawer: true,
  },
  render: () => (
    <Apps />
  ),
}, {
  name: 'tool',
  path: '/tool/:tool_id',
  meta: {
    drawer: false,
  },
  render: () => (
    <Tool />
  ),
}, {
  name: 'app',
  path: '/app/:app_id',
  meta: {
    drawer: false,
  },
  render: () => (
    <App />
  ),
}, {
  name: 'session',
  path: '/session/:session_id',
  meta: {
    drawer: true,
    topbar: false,
  },
  render: () => (
    <Session />
  ),
}, {
  name: 'dashboard',
  path: '/dashboard',
  meta: {
    drawer: true,
    background: '#ffffff'
  },
  render: () => (
    <Dashboard />
  ),
}, {
  name: 'account',
  path: '/account',
  meta: {
    drawer: true,
  },
  render: () => <Account />,
}, {
  name: 'api-reference',
  path: '/api-reference',
  meta: {
    drawer: false,
  },
  render: () => <OpenAPI />,
}, NOT_FOUND_ROUTE]

export const router = createRouter(routes, {
  defaultRoute: 'notfound',
  queryParamsMode: 'loose',
})

router.usePlugin(browserPlugin())
router.subscribe((state) => {
  const win = (window as any)
  if(win.viewPage) {
    win.viewPage(state)
  }
})
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