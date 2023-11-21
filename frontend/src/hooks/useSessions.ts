import { useContext } from 'react'

import {
  SessionsContext,
} from '../contexts/sessions'

export const useSessions = () => {
  const sessions = useContext(SessionsContext)
  return sessions
}

export default useSessions