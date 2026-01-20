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
import OrgBilling from './components/orgs/OrgBilling'
import App from './pages/App'
import Dashboard from './pages/Dashboard'
import Create from './pages/Create'
import Home from './pages/Home'
import OpenAPI from './pages/OpenAPI'
import Secrets from './pages/Secrets'
// NewAgent wizard removed - now creating blank agent and going directly to App settings
// import NewAgent from './pages/NewAgent'
import ImportAgent from './pages/ImportAgent'
import Tasks from './pages/Tasks'
import SpecTasksPage from './pages/SpecTasksPage'
import SpecTaskDetailPage from './pages/SpecTaskDetailPage'
import SpecTaskReviewPage from './pages/SpecTaskReviewPage'
import TeamDesktopPage from './pages/TeamDesktopPage'
import Projects from './pages/Projects'
import ProjectSettings from './pages/ProjectSettings'
import { FilestoreContextProvider } from './contexts/filestore'
import Files from './pages/Files'
import Fleet from './pages/Fleet'
import QuestionSets from './pages/QuestionSets'
import QuestionSetResults from './pages/QuestionSetResults'
import GitRepos from './pages/GitRepos'
import GitRepoDetail from './pages/GitRepoDetail'
import OAuthConnectionsPage from './pages/OAuthConnectionsPage'
import PasswordReset from './pages/PasswordReset'
import PasswordResetComplete from './pages/PasswordResetComplete'
import DesignDocPage from './pages/DesignDocPage'

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
      drawer: false,
      orgRouteAware: true,
    },
    render: () => (
      <Apps />
    ),
  }, {
    name: namePrefix + 'git-repos',
    path: routePrefix + '/git-repos',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Git Repositories',
    },
    render: () => {
      // Redirect to Projects page with repositories tab
      const { navigate } = useRouter()
      React.useEffect(() => {
        navigate('projects', { tab: 'repositories' }, { replace: true })
      }, [])
      return null
    },
  }, {
    name: namePrefix + 'git-repo-detail',
    path: routePrefix + '/git-repos/:repoId',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Repository',
    },
    render: () => (
      <GitRepoDetail />
    ),
  }, {
    name: namePrefix + 'qa',
    path: routePrefix + '/qa',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Q&A',
    },
    render: () => (
      <QuestionSets />
    ),
  },
  {
    name: namePrefix + 'providers',
    path: routePrefix + '/providers',
    meta: {
      drawer: false,
    },
    render: () => (
      <Providers />
    ),
  },{
    name: namePrefix + 'tasks',
    path: routePrefix + '/tasks',
    meta: {
      drawer: false,
      orgRouteAware: true,
    },
    render: () => (
      <Tasks />
    ),
  }, {
    name: namePrefix + 'spec-tasks',
    path: routePrefix + '/spec-tasks',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'SpecTasks',
    },
    render: () => (
      <SpecTasksPage />
    ),
  }, {
    name: namePrefix + 'projects',
    path: routePrefix + '/projects',
    meta: {
      drawer: true,
      orgRouteAware: true,
      title: 'Projects',
    },
    render: () => (
      <Projects />
    ),
  }, {
    name: namePrefix + 'project-specs',
    path: routePrefix + '/projects/:id/specs',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Project Tasks',
    },
    render: () => (
      <SpecTasksPage />
    ),
  }, {
    name: namePrefix + 'project-task-detail',
    path: routePrefix + '/projects/:id/tasks/:taskId',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Task Details',
    },
    render: () => (
      <SpecTaskDetailPage />
    ),
  }, {
    name: namePrefix + 'project-task-review',
    path: routePrefix + '/projects/:id/tasks/:taskId/review/:reviewId',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Spec Review',
    },
    render: () => (
      <SpecTaskReviewPage />
    ),
  }, {
    name: namePrefix + 'project-team-desktop',
    path: routePrefix + '/projects/:id/desktop/:sessionId',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Team Desktop',
    },
    render: () => (
      <TeamDesktopPage />
    ),
  }, {
    name: namePrefix + 'project-settings',
    path: routePrefix + '/projects/:id/settings',
    meta: {
      drawer: false,
      orgRouteAware: true,
      title: 'Project Settings',
    },
    render: () => (
      <ProjectSettings />
    ),
  }, {
    name: namePrefix + 'project-session',
    path: routePrefix + '/projects/:id/session/:session_id',
    meta: {
      drawer: true,
      topbar: false,
      orgRouteAware: true,
      title: 'Project Session',
    },
    render: () => (
      <Session />
    ),
  }, {
    name: namePrefix + 'app',
    path: routePrefix + '/app/:app_id',
    meta: {
      drawer: true,
    },
    render: () => (
      <App />
    ),
  }, {
    // NewAgent wizard removed - Apps.tsx now creates blank agent and navigates to App settings
    // Keeping route for backwards compatibility (redirects to apps list)
    name: namePrefix + 'new-agent',
    path: routePrefix + '/new-agent',
    meta: {
      drawer: false,
    },
    render: () => (
      <Apps />
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
  },  {
    name: namePrefix + 'qa-results',
    path: routePrefix + '/qa-results/:question_set_id/:execution_id',
    meta: {
      drawer: true,
      topbar: false,
    },
    render: () => (
      <QuestionSetResults />
    ),
  }]
}

const routes: IApplicationRoute[] = [
  ...getOrgRoutes(),
  ...getOrgRoutes('org_', '/org/:org_id'),
{
  name: 'import-agent',
  path: '/import-agent',
  meta: {
    drawer: false,
    title: 'Import Agent',
  },
  render: () => (
    <ImportAgent />
  ),
}, {
  name: 'orgs',
  path: '/orgs',
  meta: {
    drawer: false,
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
  name: 'org_billing',
  path: '/orgs/:org_id/billing',
  meta: {
    drawer: true,
    menu: 'orgs',
  },
  render: () => (
    <OrgBilling />
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
    drawer: false,
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
  name: 'account',
  path: '/account',
  meta: {
    drawer: false,
  },
  render: () => <Account />,
}, {
  name: 'api-reference',
  path: '/api-reference',
  meta: {
    drawer: false,
  },
  render: () => <OpenAPI />,
}, {
  name: 'password-reset',
  path: '/password-reset',
  meta: {
    drawer: false,
    title: 'Reset Password',
  },
  render: () => <PasswordReset />,
}, {
  name: 'password-reset-complete',
  path: '/password-reset-complete',
  meta: {
    drawer: false,
    title: 'Set New Password',
  },
  render: () => <PasswordResetComplete />,
}, {
  name: 'design-doc',
  path: '/design-doc/:specTaskId/:reviewId',
  meta: {
    drawer: false,
    title: 'Design Document',
  },
  render: () => <DesignDocPage />,
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