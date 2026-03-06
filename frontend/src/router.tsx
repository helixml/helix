import React from 'react'
import createRouter, { Route } from 'router5'
import { useRoute } from 'react-router5'
import browserPlugin from 'router5-plugin-browser'

import Session from './pages/Session'
import Apps from './pages/Apps'
import Providers from './pages/Providers'
import Orgs from './pages/Orgs'
import OrgSettings from './pages/OrgSettings'
import OrgTeams from './pages/OrgTeams'
import OrgPeople from './pages/OrgPeople'
import TeamPeople from './pages/TeamPeople'
import OrgBilling from './components/orgs/OrgBilling'
import App from './pages/App'
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
import QuestionSets from './pages/QuestionSets'
import QuestionSetResults from './pages/QuestionSetResults'
import GitRepoDetail from './pages/GitRepoDetail'
import PasswordReset from './pages/PasswordReset'
import PasswordResetComplete from './pages/PasswordResetComplete'
import DesignDocPage from './pages/DesignDocPage'
import Onboarding from './pages/Onboarding'
import Waitlist from './pages/Waitlist'
import useRouter from './hooks/useRouter'

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


const routes: IApplicationRoute[] = [
{
  name: 'org_projects',
  path: '/orgs/:org_id',
  meta: {
    title: 'Projects',
    drawer: true,
  },
  render: () => (
    <Projects />
  ),
}, {
  name: 'org_chat',
  path: '/orgs/:org_id/chat',
  meta: {
    title: 'Chat',
    drawer: true,
  },
  render: () => (
    <Home />
  ),
}, {
  name: 'org_new',
  path: '/orgs/:org_id/new',
  meta: {
    title: 'New Session',
    drawer: true,
  },
  render: () => (
    <Create />
  ),
}, {
  name: 'org_apps',
  path: '/orgs/:org_id/apps',
  meta: {
    drawer: false,
  },
  render: () => (
    <Apps />
  ),
}, {
  name: 'org_git-repos',
  path: '/orgs/:org_id/git-repos',
  meta: {
    drawer: false,
    title: 'Git Repositories',
  },
  render: () => {
    // Redirect to Projects page with repositories tab
    const { navigateReplace, params } = useRouter()
    React.useEffect(() => {
      navigateReplace('org_projects', { org_id: params.org_id, tab: 'repositories' })
    }, [])
    return null
  },
}, {
  name: 'org_git-repo-detail',
  path: '/orgs/:org_id/git-repos/:repoId',
  meta: {
    drawer: false,
    title: 'Repository',
  },
  render: () => (
    <GitRepoDetail />
  ),
}, {
  name: 'org_qa',
  path: '/orgs/:org_id/qa',
  meta: {
    drawer: false,
    title: 'Q&A',
  },
  render: () => (
    <QuestionSets />
  ),
}, {
  name: 'org_providers',
  path: '/orgs/:org_id/providers',
  meta: {
    drawer: false,
  },
  render: () => (
    <Providers />
  ),
}, {
  name: 'org_tasks',
  path: '/orgs/:org_id/tasks',
  meta: {
    drawer: false,
  },
  render: () => (
    <Tasks />
  ),
}, {
  name: 'org_spec-tasks',
  path: '/orgs/:org_id/spec-tasks',
  meta: {
    drawer: false,
    title: 'SpecTasks',
  },
  render: () => (
    <SpecTasksPage />
  ),
}, {
  name: 'org_project-specs',
  path: '/orgs/:org_id/projects/:id/specs',
  meta: {
    drawer: false,
    title: 'Project Tasks',
  },
  render: () => (
    <SpecTasksPage />
  ),
}, {
  name: 'org_project-task-detail',
  path: '/orgs/:org_id/projects/:id/tasks/:taskId',
  meta: {
    drawer: false,
    title: 'Task Details',
  },
  render: () => (
    <SpecTaskDetailPage />
  ),
}, {
  name: 'org_project-task-review',
  path: '/orgs/:org_id/projects/:id/tasks/:taskId/review/:reviewId',
  meta: {
    drawer: false,
    title: 'Spec Review',
  },
  render: () => (
    <SpecTaskReviewPage />
  ),
}, {
  name: 'org_project-team-desktop',
  path: '/orgs/:org_id/projects/:id/desktop/:sessionId',
  meta: {
    drawer: false,
    title: 'Human Desktop',
  },
  render: () => (
    <TeamDesktopPage />
  ),
}, {
  name: 'org_project-settings',
  path: '/orgs/:org_id/projects/:id/settings',
  meta: {
    drawer: false,
    title: 'Project Settings',
  },
  render: () => (
    <ProjectSettings />
  ),
}, {
  name: 'org_project-session',
  path: '/orgs/:org_id/projects/:id/session/:session_id',
  meta: {
    drawer: true,
    topbar: false,
    title: 'Project Session',
  },
  render: () => (
    <Session />
  ),
}, {
  name: 'org_app',
  path: '/orgs/:org_id/app/:app_id',
  meta: {
    drawer: true,
  },
  render: () => (
    <App />
  ),
}, {
  name: 'org_new-agent',
  path: '/orgs/:org_id/new-agent',
  meta: {
    drawer: false,
  },
  render: () => (
    <Apps />
  ),
}, {
  name: 'org_session',
  path: '/orgs/:org_id/session/:session_id',
  meta: {
    drawer: true,
    topbar: false,
  },
  render: () => (
    <Session />
  ),
}, {
  name: 'org_qa-results',
  path: '/orgs/:org_id/qa-results/:question_set_id/:execution_id',
  meta: {
    drawer: true,
    topbar: false,
  },
  render: () => (
    <QuestionSetResults />
  ),
}, {
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
}, {
  name: 'onboarding',
  path: '/onboarding',
  meta: {
    drawer: false,
    fullscreen: true,
    title: 'Get Started',
  },
  render: () => <Onboarding />,
}, {
  name: 'waitlist',
  path: '/waitlist',
  meta: {
    drawer: false,
    fullscreen: true,
    title: 'Waitlist',
  },
  render: () => <Waitlist />,
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

const SELECTED_ORG_STORAGE_KEY = 'selected_org'

const getStoredOrg = (): string | undefined => {
  const currentPath = window.location.pathname
  if (currentPath !== '/' && currentPath !== '') return undefined

  const storedOrg = localStorage.getItem(SELECTED_ORG_STORAGE_KEY)
  if (!storedOrg) return undefined

  return storedOrg
}

const storedOrg = getStoredOrg()
router.start()

if (storedOrg) {
  router.navigate('org_projects', { org_id: storedOrg }, { replace: true })
}

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
