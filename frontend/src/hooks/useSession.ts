import { useContext } from 'react'

import {
  SessionContext,
} from '../contexts/session'

export const useSession = () => {
  const session = useContext(SessionContext)
  return session
}

export default useSession