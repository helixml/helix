import { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'

import useRouter from '../hooks/useRouter'
import { useActivateBot, useListHelixOrgBots } from '../services/helixOrgService'

export default function OrgBotSessionResolver() {
  const router = useRouter()
  const orgID = router.params.org_id || ''
  const botID = router.params.bot_id || ''
  const attemptedBot = useRef('')
  const [activationError, setActivationError] = useState(false)
  const {
    data: bots = [],
    isLoading,
    isError: listError,
    refetch,
  } = useListHelixOrgBots({
    enabled: !!orgID && !!botID,
    refetchInterval: 2000,
  })
  const bot = bots.find((candidate) => candidate.id === botID)
  const sessionID = bot?.session_id || ''
  const activateBot = useActivateBot(orgID)

  useEffect(() => {
    if (!orgID || !sessionID) return
    router.navigateReplace('org_session', {
      org_id: orgID,
      session_id: sessionID,
    })
  }, [orgID, sessionID]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const botKey = `${orgID}:${botID}`
    if (!bot?.id || sessionID || attemptedBot.current === botKey) return
    attemptedBot.current = botKey
    setActivationError(false)
    activateBot.mutateAsync(botID).catch(() => setActivationError(true))
  }, [bot?.id, botID, orgID, sessionID]) // eslint-disable-line react-hooks/exhaustive-deps

  const retryActivation = () => {
    setActivationError(false)
    activateBot.mutateAsync(botID).catch(() => setActivationError(true))
  }

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 2,
      }}
    >
      {listError ? (
        <>
          <Typography color="error" role="alert">
            Could not find your Chief of Staff.
          </Typography>
          <Button
            variant="contained"
            onClick={() => void refetch()}
            aria-label="Retry finding Chief of Staff"
          >
            Retry
          </Button>
        </>
      ) : activationError ? (
        <>
          <Typography color="error" role="alert">
            Could not start your Chief of Staff.
          </Typography>
          <Button
            variant="contained"
            onClick={retryActivation}
            aria-label="Retry starting Chief of Staff"
            disabled={activateBot.isPending}
          >
            Retry
          </Button>
        </>
      ) : (
        <>
          <CircularProgress size={24} />
          <Typography color="text.secondary">
            {isLoading || !bot
              ? 'Finding your Chief of Staff...'
              : 'Starting your Chief of Staff...'}
          </Typography>
          {bot && !sessionID && (
            <Button
              variant="outlined"
              onClick={retryActivation}
              aria-label="Retry starting Chief of Staff"
              disabled={activateBot.isPending}
            >
              Retry
            </Button>
          )}
        </>
      )}
    </Box>
  )
}
