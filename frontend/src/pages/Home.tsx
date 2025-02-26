import React, { FC, useState, useCallback, KeyboardEvent } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import AddIcon from '@mui/icons-material/Add'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import Tooltip from '@mui/material/Tooltip'

import Page from '../components/system/Page'
import Row from '../components/widgets/Row'
import SessionTypeButton from '../components/create/SessionTypeButton'
import ModelPicker from '../components/create/ModelPicker'
import ExamplePrompts from '../components/create/ExamplePrompts'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import { ISessionType, SESSION_TYPE_TEXT } from '../types'

import useLightTheme from '../hooks/useLightTheme'
import useIsBigScreen from '../hooks/useIsBigScreen'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useSessions from '../hooks/useSessions'
import { useStreaming } from '../contexts/streaming'

const Home: FC = () => {
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const snackbar = useSnackbar()
  const sessions = useSessions()
  const { NewInference } = useStreaming()
  const [currentPrompt, setCurrentPrompt] = useState('')
  const [currentMode, setCurrentMode] = useState<ISessionType>(SESSION_TYPE_TEXT)
  const [currentModel, setCurrentModel] = useState<string>('')
  const [loading, setLoading] = useState(false)

  const submitPrompt = useCallback(async () => {
    if (!currentPrompt.trim()) return
    setLoading(true)

    try {
      const session = await NewInference({
        type: currentMode,
        message: currentPrompt,
        modelName: currentModel,
      })

      if (!session) return
      await sessions.loadSessions()
      setLoading(false)
      router.navigate('session', { session_id: session.id })
    } catch (error) {
      console.error('Error in submitPrompt:', error)
      snackbar.error('Failed to start inference')
      setLoading(false)
    }
  }, [currentPrompt, currentMode, currentModel, NewInference, sessions, router, snackbar])

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter') {
      if (!e.shiftKey) {
        e.preventDefault()
        submitPrompt()
      }
    }
  }

  return (
    <Page
      showTopbar={ isBigScreen ? false : true }
    >
      <Box
        sx={{
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {/* Main content */}
        <Box
          sx={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            overflow: 'hidden',
          }}
        >
          <Container
            maxWidth="xl"
            sx={{
              height: '100%',
              overflowY: 'auto',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <Grid container spacing={ 2 } justifyContent="center">
              <Grid item xs={ 12 } sm={ 12 } md={ 12 } lg={ 6 } sx={{ textAlign: 'center' }}>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Typography
                    sx={{
                      color: '#fff',
                      fontSize: '1.5rem',
                      fontWeight: 'bold',
                      textAlign: 'center',
                      mb: 2,
                    }}
                  >
                    How can I help?
                  </Typography>
                </Row>
                <Row>
                  <Box
                    sx={{
                      width: '100%',
                      border: '1px solid rgba(255, 255, 255, 0.2)',
                      borderRadius: '12px',
                      backgroundColor: 'rgba(255, 255, 255, 0.05)',
                      p: 2,
                      mb: 2,
                    }}
                  >
                    {/* Top row - Chat with Helix */}
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        mb: 2,
                      }}
                    >
                      <textarea
                        value={currentPrompt}
                        onChange={(e) => setCurrentPrompt(e.target.value)}
                        onKeyDown={handleKeyDown}
                        rows={2}
                        style={{
                          width: '100%',
                          backgroundColor: 'transparent',
                          border: 'none',
                          color: '#fff',
                          opacity: 0.7,
                          resize: 'none',
                          outline: 'none',
                          fontFamily: 'inherit',
                          fontSize: 'inherit',
                        }}
                        placeholder="Chat with Helix"
                      />
                    </Box>

                    {/* Bottom row - Split into left and right sections */}
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                      }}
                    >
                      {/* Left section - Will contain SessionTypeButton, ModelPicker and plus button */}
                      <Box
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 1,
                        }}
                      >
                        <SessionTypeButton 
                          type={currentMode}
                          onSetType={setCurrentMode}
                        />
                        <ModelPicker
                          type={currentMode}
                          model={currentModel}
                          provider={undefined}
                          displayMode="short"
                          border
                          compact
                          onSetModel={setCurrentModel}
                        />
                        {/* Plus button */}
                        <Tooltip title="Add Documents" placement="top">
                          <Box 
                            sx={{ 
                              width: 32, 
                              height: 32,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              cursor: 'pointer',
                              border: '2px solid rgba(255, 255, 255, 0.7)',
                              borderRadius: '50%',
                              '&:hover': {
                                borderColor: 'rgba(255, 255, 255, 0.9)',
                                '& svg': {
                                  color: 'rgba(255, 255, 255, 0.9)'
                                }
                              }
                            }}
                          >
                            <AddIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                          </Box>
                        </Tooltip>
                      </Box>

                      {/* Right section - Up arrow icon */}
                      <Box>
                        <Tooltip title="Send Prompt" placement="top">
                          <Box 
                            onClick={submitPrompt}
                            sx={{ 
                              width: 32, 
                              height: 32,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              cursor: loading ? 'default' : 'pointer',
                              border: '1px solid rgba(255, 255, 255, 0.7)',
                              borderRadius: '8px',
                              opacity: loading ? 0.5 : 1,
                              '&:hover': loading ? {} : {
                                borderColor: 'rgba(255, 255, 255, 0.9)',
                                '& svg': {
                                  color: 'rgba(255, 255, 255, 0.9)'
                                }
                              }
                            }}
                          >
                            {loading ? (
                              <LoadingSpinner />
                            ) : (
                              <ArrowUpwardIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                            )}
                          </Box>
                        </Tooltip>
                      </Box>
                    </Box>
                  </Box>
                </Row>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <ExamplePrompts
                    header={false}
                    layout="vertical"
                    type={currentMode}
                    onChange={setCurrentPrompt}
                  />
                </Row>
              </Grid>
            </Grid>
          </Container>
        </Box>

        {/* Footer */}
        <Box
          sx={{
            py: 2,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
          }}
        >
          <Typography
            sx={{
              color: lightTheme.textColorFaded,
              fontSize: '0.8rem',
            }}
          >
            Open source models can make mistakes. Check facts, dates and events.
          </Typography>
        </Box>
      </Box>
    </Page>
  )
}

export default Home