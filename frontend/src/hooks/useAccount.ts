import { useContext } from 'react'

import {
  AccountContext,
} from '../contexts/account'

export const useAccount = () => {
  const account = useContext(AccountContext)
  return account
}

export default useAccount