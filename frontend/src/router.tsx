import createRouter, { Route } from 'router5'
import { useRoute } from 'react-router5'
import browserPlugin from 'router5-plugin-browser'

import Session from './pages/Session'
import Account from './pages/Account'
import Apps from './pages/Apps'
import Providers from './pages/Providers'
import Orgs from './pages/Orgs'
import OrgSettings from './pages/OrgSettings'
import OrgTeams from './pages/OrgTeams'
import OrgPeople from './pages/OrgPeople'
import TeamPeople from './pages/TeamPeople'
import App from './pages/App'
import Dashboard from './pages/Dashboard'
import Create from './pages/Create'
import Home from './pages/Home'
import OpenAPI from './pages/OpenAPI'
import Secrets from './pages/Secrets'
import NewAgent from './pages/NewAgent'
import { FilestoreContextProvider } from './contexts/filestore'
import Files from './pages/Files'
import OAuthConnectionsPage from './pages/OAuthConnectionsPage'

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


// some routes work for both the `/org/:org_id/` prefix and also for the root prefix
// so rather than duplicate these routes let's return them from this utility function
const getOrgRoutes = (namePrefix = '', routePrefix = ''): IApplicationRoute[] => {
  return [{
    name: namePrefix + 'home',
    path: routePrefix + (routePrefix ? '' : '/'),
    meta: {
      title: 'Home',
      drawer: true,
      orgRouteAware: true,
    },
    render: () => (
        <Home />
    ),
  }, {
    name: namePrefix + 'new',
    path: routePrefix + '/new',
    meta: {
      title: 'New Session',
      drawer: true,
      orgRouteAware: true,
    },
    render: () => (
        <Create />
    ),
  }, {
    name: namePrefix + 'apps',
    path: routePrefix + '/apps',
    meta: {
      drawer: true,
      orgRouteAware: true,
    },
    render: () => (
      <Apps />
    ),
  }, {
    name: namePrefix + 'app',
    path: routePrefix + '/app/:app_id',
    meta: {
      drawer: false,
    },
    render: () => (
      <App />
    ),
  }, {
    name: namePrefix + 'new-agent',
    path: routePrefix + '/new-agent',
    meta: {
      drawer: true,
    },
    render: () => (
      <NewAgent />
    ),
  }, {
    name: namePrefix + 'session',
    path: routePrefix + '/session/:session_id',
    meta: {
      drawer: true,
      topbar: false,
    },
    render: () => (
      <Session />
    ),
  }]
}

const routes: IApplicationRoute[] = [
  ...getOrgRoutes(),
  ...getOrgRoutes('org_', '/org/:org_id'),
{
  name: 'orgs',
  path: '/orgs',
  meta: {
    drawer: true,
  },
  render: () => (
    <Orgs />
  ),
}, {
  name: 'org_settings',
  path: '/orgs/:org_id/settings',
  meta: {
    drawer: true,
    menu: 'orgs',
  },
  render: () => (
    <OrgSettings />
  ),
}, {
  name: 'org_people',
  path: '/orgs/:org_id/people',
  meta: {
    drawer: true,
    menu: 'orgs',
  },
  render: () => (
    <OrgPeople />
  ),
}, {
  name: 'org_teams',
  path: '/orgs/:org_id/teams',
  meta: {
    drawer: true,
    menu: 'orgs',
  },
  render: () => (
    <OrgTeams />
  ),
}, {
  name: 'team_people',
  path: '/orgs/:org_id/teams/:team_id/people',
  meta: {
    drawer: true,
    menu: 'orgs',
    orgRouteName: 'org_teams',
  },
  render: () => (
    <TeamPeople />
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
  name: 'secrets',
  path: '/secrets',
  meta: {
    drawer: true,
  },
  render: () => (
    <Secrets />
  ),
}, {
  name: 'oauth-connections',
  path: '/oauth-connections',
  meta: {
    drawer: true,
    title: 'Connected Services',
  },
  render: () => (
    <OAuthConnectionsPage />
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
  name: 'user-providers',
  path: '/providers',
  meta: {
    drawer: true,
  },
  render: () => (
    <Providers />
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