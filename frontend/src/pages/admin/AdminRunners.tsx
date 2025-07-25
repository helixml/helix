import React, { FC, useState, useRef, useCallback, useEffect } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import Switch from '@mui/material/Switch'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import LaunchIcon from '@mui/icons-material/Launch'

import Page from '../../components/system/Page'
import RunnerSummary from '../../components/session/RunnerSummary'
import SessionSummary from '../../components/session/SessionSummary'
import SchedulingDecisionsTable from '../../components/dashboard/SchedulingDecisionsTable'
import JsonWindowLink from '../../components/widgets/JsonWindowLink'
import Window from '../../components/widgets/Window'
import SessionToolbar from '../../components/session/SessionToolbar'
import Interaction from '../../components/session/Interaction'

import { useGetDashboardData } from '../../services/dashboardService'
import { useFloatingRunnerState } from '../../contexts/floatingRunnerState'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'

import { ISession, ISessionSummary } from '../../types'
import { TypesWorkloadSummary, TypesDashboardRunner } from '../../api/api'

const START_ACTIVE = true

const AdminRunners: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const api = useApi()
  const floatingRunnerState = useFloatingRunnerState()

  const activeRef = useRef(START_ACTIVE)
  const [viewingSession, setViewingSession] = useState<ISession>()
  const [active, setActive] = useState(START_ACTIVE)

  const { session_id } = router.params
  const { data: dashboardData, isLoading: isLoadingDashboardData } = useGetDashboardData()

  const onViewSession = useCallback((session_id: string) => {
    router.setParams({ session_id })
  }, [router])

  const onCloseViewingSession = useCallback(() => {
    setViewingSession(undefined)
    router.removeParams(['session_id'])
  }, [router])

  useEffect(() => {
    if (!session_id) return
    if (!account.user) return
    const loadSession = async () => {
      const session = await api.get<ISession>(`/api/v1/sessions/${session_id}`)
      if (!session) return
      setViewingSession(session)
    }
    loadSession()
  }, [account.user, session_id, api])

  if (!account.user) return null
  if (isLoadingDashboardData) return null

  return (
    <Page breadcrumbTitle="Runners">
      <Container maxWidth="xl" sx={{ mt: 2, height: 'calc(100% - 50px)' }}>
        <Box
          sx={{
            width: '100%',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-start',
            justifyContent: 'flex-start',
          }}
        >
          {/* Controls for entire Runners section */}
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'space-between',
              mb: 3,
              px: 2,
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <FormGroup>
                <FormControlLabel
                  control={
                    <Switch
                      checked={active}
                      onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                        activeRef.current = event.target.checked
                        setActive(event.target.checked)
                      }}
                    />
                  }
                  label="Live Updates?"
                />
              </FormGroup>
              
              {account.admin && (
                <Tooltip title="Toggle floating runner state view (Ctrl/Cmd+Shift+S)" arrow>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={<LaunchIcon />}
                    onClick={floatingRunnerState.toggleFloatingRunnerState}
                    sx={{
                      borderColor: 'rgba(0, 200, 255, 0.3)',
                      color: floatingRunnerState.isVisible ? '#00c8ff' : 'rgba(255, 255, 255, 0.7)',
                      backgroundColor: floatingRunnerState.isVisible ? 'rgba(0, 200, 255, 0.1)' : 'transparent',
                      '&:hover': {
                        borderColor: 'rgba(0, 200, 255, 0.5)',
                        backgroundColor: 'rgba(0, 200, 255, 0.1)',
                      }
                    }}
                  >
                    {floatingRunnerState.isVisible ? 'Hide' : 'Show'} Floating View
                  </Button>
                </Tooltip>
              )}
            </Box>
            
            <JsonWindowLink data={dashboardData}>
              view data
            </JsonWindowLink>
          </Box>

          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'flex-start',
              justifyContent: 'flex-start',
            }}
          >
            {/* Queue Section */}
            <Box
              sx={{
                p: 3,
                flexGrow: 0,
                width: '480px',
                minWidth: '480px',
                overflowY: 'auto',
                display: { xs: 'none', md: 'block' },
                borderRight: '1px solid rgba(255, 255, 255, 0.08)',
              }}
            >
              {/* Queue Section Header */}
              <Box
                sx={{
                  mb: 3,
                  pb: 1,
                  borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
                }}
              >
                <Typography 
                  variant="h5"
                  sx={{ 
                    color: 'rgba(255, 255, 255, 0.95)',
                    fontWeight: 600,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
                  }}
                >
                  Queue
                  {dashboardData && dashboardData.queue && dashboardData.queue.length > 0 && (
                    <Chip
                      size="small"
                      label={dashboardData.queue.length}
                      sx={{
                        height: 22,
                        minWidth: 20,
                        backgroundColor: 'rgba(128, 90, 213, 0.15)',
                        color: 'rgba(255, 255, 255, 0.7)',
                        border: '1px solid rgba(128, 90, 213, 0.3)',
                        '& .MuiChip-label': {
                          px: 1,
                          fontSize: '0.7rem',
                          fontWeight: 600,
                        }
                      }}
                    />
                  )}
                </Typography>
              </Box>

              {dashboardData && dashboardData?.queue?.length === 0 && (
                <Box sx={{ 
                  py: 4, 
                  textAlign: 'center',
                  backgroundColor: 'rgba(25, 25, 28, 0.3)',
                  borderRadius: '3px',
                }}>
                  <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                    No jobs in queue
                  </Typography>
                </Box>
              )}
              
              {dashboardData && dashboardData?.queue?.map((item: TypesWorkloadSummary) => {
                return (
                  <SessionSummary
                    key={item.id}
                    session={
                      {
                        session_id: item.id,
                        created: item.created,
                        updated: item.updated,
                        model_name: item.model_name,
                        mode: item.mode,
                        type: item.runtime,
                        owner: "todo",
                        lora_dir: item.lora_dir,
                        summary: item.summary,
                        app_id: "todo",
                      } as ISessionSummary
                    }
                    onViewSession={onViewSession}
                  />
                )
              })}             
            </Box>

            {/* Runners Section */}
            <Box
              sx={{
                flexGrow: 1,
                p: 3,
                height: '100%',
                width: '100%',
                overflowY: 'auto',
              }}
            >
              {/* Runners Section Header */}
              <Typography 
                variant="h5"
                sx={{ 
                  mb: 4,
                  color: 'rgba(255, 255, 255, 0.95)',
                  fontWeight: 600,
                  borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
                  pb: 1,
                }}
              >
                Runner State
              </Typography>

              <Grid
                container
                sx={{
                  width: '100%',
                  overflow: "auto",
                }}
                spacing={3}
              >
                {dashboardData && dashboardData?.runners?.length === 0 && (
                  <Grid item xs={12}>
                    <Box sx={{ 
                      py: 4, 
                      textAlign: 'center',
                      backgroundColor: 'rgba(25, 25, 28, 0.3)',
                      borderRadius: '3px',
                    }}>
                      <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                        No active runners
                      </Typography>
                    </Box>
                  </Grid>
                )}

                {dashboardData && dashboardData?.runners?.map((runner: TypesDashboardRunner) => {
                  return (
                    <Grid item xs={12} key={runner.id}>
                      <RunnerSummary
                        runner={runner}
                        onViewSession={onViewSession} 
                      />
                    </Grid>
                  )
                })}
              </Grid>
              
              {/* Scheduling Decisions Section */}
              <Box sx={{ mt: 4 }}>
                <Typography 
                  variant="h5"
                  sx={{ 
                    mb: 3,
                    color: 'rgba(255, 255, 255, 0.95)',
                    fontWeight: 600,
                    borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
                    pb: 1,
                  }}
                >
                  Recent Scheduling Decisions
                </Typography>
                
                <Box sx={{ 
                  mb: 2,
                  color: 'rgba(255, 255, 255, 0.6)',
                  fontSize: '0.875rem',
                }}>
                  Live log of decisions made by the central scheduler when assigning workloads to runners
                </Box>
                
                <SchedulingDecisionsTable 
                  decisions={dashboardData?.scheduling_decisions || []} 
                />
              </Box>
            </Box>
          </Box>
        </Box>

        {viewingSession && (
          <Window
            open
            size="lg"
            background="#FAEFE0"
            withCancel
            cancelTitle="Close"
            onCancel={onCloseViewingSession}
          >
            <SessionToolbar session={viewingSession} />
            {viewingSession.interactions.map((interaction: any, i: number) => {
              return (
                <Interaction
                  key={i}
                  showFinetuning={true}
                  serverConfig={account.serverConfig}
                  interaction={interaction}
                  session={viewingSession}
                  onRegenerate={() => {}}
                  retryFinetuneErrors={() => {}}
                  onReloadSession={async () => {}}
                  onClone={async () => false}
                  isOwner={true}
                  isAdmin={false}
                  session_id={viewingSession.id}
                  highlightAllFiles={false}
                  isLastInteraction={i === viewingSession.interactions.length - 1}
                  hasSubscription={true}
                />
              )
            })}
          </Window>
        )}
      </Container>
    </Page>
  )
}

export default AdminRunners 