import React, { FC, useEffect } from 'react'
import Typography from '@mui/material/Typography'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useSession from '../../hooks/useSession'

// GLOBAL component
// it uses the router context to load a session summary
// so it can render the title
export const SessionTitle: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const session = useSession()
  
  const sessionID = router.params.session_id

  useEffect(() => {
    if(!account.user) return
    if(sessionID) {
      session.loadSessionSummary(sessionID)
    }
  }, [
    account.user,
    sessionID,
  ])

  return (
    <Typography
      component="h1"
      variant="h6"
      color="inherit"
      noWrap
      sx={{
        flexGrow: 1,
        ml: 1,
        color: 'text.primary',
      }}
    >
      { session.summary?.name }
    </Typography>               
  )
}

export default SessionTitle