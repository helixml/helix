import createRouter, { Route } from 'router5'
import { useRoute } from 'react-router5'
import browserPlugin from 'router5-plugin-browser'
import Box from '@mui/material/Box'

import Session from './pages/Session'
import Account from './pages/Account'
import Tools from './pages/Tools'
import Tool from './pages/Tool'
import Apps from './pages/Apps'
import App from './pages/App'
import Dashboard from './pages/Dashboard'
import New from './pages/New'
import Collection from './pages/Collection'
import ImageFineTuneView from './pages/ImageFineTuneView'
import ImageFineTuneMoreView from './pages/ImageFineTuneMoreView'
import TextFineTuneUpdate from './pages/TextFineTuneUpdate'
import TextFineTuneViewQuestions from './pages/TextFineTuneViewQuestions '


import CollectionTitle from './components/collection/CollectionTitle'
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

const routes: IApplicationRoute[] = [
  {
  name: 'new',
  path: '/',
  meta: {
    title: 'The start of something beautiful',
    sidebar: true,
  },
  render: () => (
      <New />
  ),
}, {
  name: 'create-collection',
  path: '/create-collection',
  meta: {
    title: 'Create Collection',
    sidebar: true,
  },
  render: () => (
      <New />
  ),
}, {
  name: 'collection',
  path: '/collection',
  meta: {
    title: 'Collection',
    sidebar: true,
  },
  getTitle: () => {
    return (
      <CollectionTitle />
    )
  },
  render: () => (
      <Collection />
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
  name: 'apps',
  path: '/apps',
  meta: {
    title: 'Apps',
    sidebar: true,
  },
  render: () => (
    <Apps />
  ),
}, {
  name: 'tool',
  path: '/tool/:tool_id',
  meta: {
    title: 'Edit Tool',
    sidebar: false,
  },
  render: () => (
    <Tool />
  ),
}, {
  name: 'app',
  path: '/app/:app_id',
  meta: {
    title: 'Edit App',
    sidebar: false,
  },
  render: () => (
    <App />
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
}, {
  name: 'imagefinetuneview',
  path: '/imagefinetuneview',
  meta: {
    title: 'A Dogs Dinner',
    sidebar: true,
  },
  render: () => (
      <ImageFineTuneView />
  ),
}, {
  name: 'imagefinetunemoreview',
  path: '/imagefinetunemoreview',
  meta: {
    title: 'A Dogs Dinner',
    sidebar: true,
  },
  render: () => (
      <ImageFineTuneMoreView />
  ),
}, 
{
  name: 'textfinetuneupdate',
  path: '/textfinetuneupdate',
  meta: {
    title: 'A Dogs Dinner',
    sidebar: true,
  },
  render: () => (
      <TextFineTuneUpdate />
  ),
}, 
{
  name: 'textfinetuneviewquestions',
  path: '/textfinetuneviewquestions',
  meta: {
    title: 'A Dogs Dinner',
    sidebar: true,
  },
  render: () => (
      <TextFineTuneViewQuestions />
  ),
}, 

NOT_FOUND_ROUTE]

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