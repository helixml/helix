import React, { FC, useMemo } from 'react'
import Typography from '@mui/material/Typography'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import { useTheme } from '@mui/material/styles'
import { Edit, Info } from 'lucide-react'

import Markdown from '../components/session/Markdown'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useThemeConfig from '../hooks/useThemeConfig'
import useLightTheme from '../hooks/useLightTheme'
import { useQuestionSet, useQuestionSetExecutionResults } from '../services/questionSetsService'

const QuestionSetResults: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const api = useApi()
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()

  const questionSetId = router.params.question_set_id
  const executionId = router.params.execution_id

  const { data: questionSet } = useQuestionSet(questionSetId || '', {
    enabled: !!questionSetId && !!executionId,
  })

  const { data: executionResults, isLoading: isLoadingResults } = useQuestionSetExecutionResults(
    questionSetId,
    executionId,
    undefined,
    { enabled: !!questionSetId && !!executionId }
  )

  if (!executionId || !questionSetId) {
    return (
      <Box
        sx={{
          width: '100%',
          height: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Typography>Invalid execution or question set ID</Typography>
      </Box>
    )
  }

  if (isLoadingResults) {
    return (
      <Box
        sx={{
          width: '100%',
          height: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <LoadingSpinner />
      </Box>
    )
  }

  return (
    <Box
      sx={{
        width: '100%',
        height: '100vh',
        display: 'flex',
        flexDirection: 'row',
      }}
    >
      <Box
        sx={{
          flexGrow: 1,
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <Box
          sx={{
            width: '100%',
            flexShrink: 0,
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder,
            py: 2,
            px: 3,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              gap: 1,
            }}
          >
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 2,
              }}
            >
              <Typography
                component="h1"
                sx={{
                  fontSize: { xs: 'small', sm: 'medium', md: 'large' },
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                Question Set Results
              </Typography>
              {questionSetId && (
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<Edit size={18} />}
                  onClick={() => {
                    account.orgNavigate('qa', {})
                    setTimeout(() => {
                      const url = new URL(window.location.href)
                      url.searchParams.set('questionSetId', questionSetId)
                      window.history.replaceState({}, '', url.toString())
                      window.dispatchEvent(new PopStateEvent('popstate'))
                    }, 100)
                  }}
                  sx={{
                    fontSize: '0.7rem',
                    py: 0.25,
                    px: 1,
                    minWidth: 'auto',
                  }}
                >
                  Edit Question Set
                </Button>
              )}
            </Box>
            {questionSet?.created && (
              <Typography variant="caption" sx={{ color: 'gray' }}>
                Created on{' '}
                <Tooltip title={new Date(questionSet.created).toLocaleString()}>
                  <Box component="span">{new Date(questionSet.created).toLocaleDateString()}</Box>
                </Tooltip>
              </Typography>
            )}
          </Box>
        </Box>

        <Box
          sx={{
            flexGrow: 1,
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0,
            overflow: 'hidden',
          }}
        >
          <Box
            sx={{
              flexGrow: 1,
              display: 'flex',
              flexDirection: 'column',
              overflowY: 'auto',
              pr: 3,
              minHeight: 0,
              ...lightTheme.scrollbar,
            }}
          >
            <Container maxWidth="lg">
              <Box
                sx={{
                  width: '100%',
                  maxWidth: 700,
                  mx: 'auto',
                  px: { xs: 1, sm: 2, md: 0 },
                  py: 2,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 4,
                }}
              >
                {!executionResults?.results || executionResults.results.length === 0 ? (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      py: 8,
                    }}
                  >
                    <Typography color="text.secondary">No results found</Typography>
                  </Box>
                ) : (
                  executionResults.results.map((res, index) => {
                    const sessionId = res.session_id
                    return (
                      <Box
                        key={res.question_id || index}
                        sx={{
                          width: '100%',
                          p: 3,
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 2,
                        }}
                      >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="h4" component="h4" sx={{ flexGrow: 1 }}>
                            {res.question}
                          </Typography>
                          {sessionId && (
                            <Tooltip title="Open session">
                              <IconButton
                                size="small"
                                onClick={() => account.orgNavigate('session', { session_id: sessionId })}
                              >
                                <Info size={16} />
                              </IconButton>
                            </Tooltip>
                          )}
                        </Box>
                        {res.response && (
                          <Box>
                            <Markdown
                              text={res.response}
                              session={undefined as any}
                              getFileURL={(url: string) => {
                                if (!url) return ''
                                if (!account.serverConfig) return ''
                                if (url.startsWith('data:')) return url
                                return `${account.serverConfig.filestore_prefix}/${url}?redirect_urls=true`
                              }}
                              showBlinker={false}
                              isStreaming={false}
                            />
                          </Box>
                        )}
                      </Box>
                    )
                  })
                )}
              </Box>
            </Container>
          </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default QuestionSetResults

