import { useContext } from 'react'

import {
  RouterContext,
} from '../contexts/router'

export const useRouter = () => {
  const router = useContext(RouterContext)
  return router
}

export default useRouter