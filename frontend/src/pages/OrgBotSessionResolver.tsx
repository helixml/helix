import { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import CheckRoundedIcon from '@mui/icons-material/CheckRounded'

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
    isError: listError,
    refetch,
  } = useListHelixOrgBots({
    enabled: !!orgID && !!botID,
    refetchInterval: 2000,
  })
  const bot = bots.find((candidate) => candidate.id === botID)
  const agentName = bot?.name || botID
  const sessionID = bot?.session_id || ''
  const activateBot = useActivateBot(orgID)
  const currentStage = sessionID ? 2 : bot ? 1 : 0
  const stages = [
    ['Finding your agent', 'Checking your organization'],
    ['Starting a secure workspace', `Preparing ${agentName}`],
    ['Opening your conversation', 'Taking you to the chat'],
  ]

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
            Could not find this agent.
          </Typography>
          <Button
            variant="contained"
            onClick={() => void refetch()}
            aria-label="Retry finding agent"
          >
            Retry
          </Button>
        </>
      ) : activationError ? (
        <>
          <Typography color="error" role="alert">
            Could not start {agentName}.
          </Typography>
          <Button
            variant="contained"
            onClick={retryActivation}
            aria-label={`Retry starting ${agentName}`}
            disabled={activateBot.isPending}
          >
            Retry
          </Button>
        </>
      ) : (
        <Box
          sx={{
            width: 'calc(100% - 32px)',
            maxWidth: 440,
            p: { xs: 3, sm: 4 },
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 3,
            bgcolor: 'background.paper',
          }}
        >
          <Typography variant="h5" sx={{ fontWeight: 650, letterSpacing: '-0.02em' }}>
            Meet {agentName}
          </Typography>
          <Typography color="text.secondary" sx={{ mt: 1, lineHeight: 1.6 }}>
            Your new agent is getting ready to work with your organization.
          </Typography>

          <Box component="ol" sx={{ listStyle: 'none', p: 0, m: 0, mt: 3 }}>
            {stages.map(([label, detail], index) => {
              const complete = index < currentStage
              const active = index === currentStage
              return (
                <Box
                  component="li"
                  key={label}
                  aria-current={active ? 'step' : undefined}
                  sx={{ display: 'flex', gap: 1.5, minHeight: 58 }}
                >
                  <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                    <Box
                      sx={{
                        width: 26,
                        height: 26,
                        borderRadius: '50%',
                        display: 'grid',
                        placeItems: 'center',
                        border: '1px solid',
                        borderColor: complete || active ? 'secondary.main' : 'divider',
                        bgcolor: complete ? 'secondary.main' : 'transparent',
                        color: complete ? 'secondary.contrastText' : 'text.secondary',
                      }}
                    >
                      {complete ? (
                        <CheckRoundedIcon sx={{ fontSize: 17 }} />
                      ) : active ? (
                        <CircularProgress size={16} color="secondary" />
                      ) : (
                        <Typography variant="caption">{index + 1}</Typography>
                      )}
                    </Box>
                    {index < stages.length - 1 && (
                      <Box sx={{ width: '1px', flex: 1, bgcolor: 'divider' }} />
                    )}
                  </Box>
                  <Box sx={{ pt: 0.25 }}>
                    <Typography sx={{ fontWeight: active ? 600 : 500, color: active ? 'text.primary' : 'text.secondary' }}>
                      {label}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {detail}
                    </Typography>
                  </Box>
                </Box>
              )
            })}
          </Box>
        </Box>
      )}
    </Box>
  )
}
