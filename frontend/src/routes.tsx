import { A } from 'hookrouter'
import Home from './pages/Home'
import Jobs from './pages/Jobs'
import Files from './pages/Files'
import Account from './pages/Account'

export type IRouteObject = {
  id: string,
  title?: string | JSX.Element,
  render: {
    (): JSX.Element,
  },
  params: Record<string, any>,
}

export type IRouteFactory = (props: Record<string, any>) => IRouteObject

export const routes: Record<string, IRouteFactory> = {
  '/': () => ({
    id: 'home',
    title: 'Modules',
    render: () => <Home />,
    params: {},
  }),
  '/jobs': () => ({
    id: 'jobs',
    title: 'Jobs',
    render: () => <Jobs />,
    params: {},
  }),
  '/files': () => ({
    id: 'files',
    title: 'Files',
    render: () => <Files />,
    params: {},
  }),
  '/account': () => ({
    id: 'account',
    title: 'Account',
    render: () => <Account />,
    params: {},
  }),
}

export default routes