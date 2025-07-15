import { FC } from 'react'

import { useListUserCronTriggers } from '../services/appService'
import useAccount from '../hooks/useAccount'

const Tasks: FC = () => {

  const account = useAccount()

  const { data: triggers, isLoading } = useListUserCronTriggers(account.organizationTools.organization?.id || '')

  return (
   
  )
}

export default Tasks
