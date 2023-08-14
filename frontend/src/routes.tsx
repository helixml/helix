import { A } from 'hookrouter'
import Home from './pages/Home'

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
    title: 'Home',
    render: () => <Home />,
    params: {},
  }),
}

export default routes