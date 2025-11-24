import React, { FC, useMemo, useEffect, useState } from 'react'
import Typography from '@mui/material/Typography'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import { useTheme } from '@mui/material/styles'
import { Edit, Info } from 'lucide-react'
import pdfIcon from '../../assets/img/pdf-icon.png'

import Markdown from '../components/session/Markdown'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import ExportDocument from '../components/export/ExportDocument'
import ToPDF from '../components/export/ToPDF'

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

  const [activeSection, setActiveSection] = useState<string>('')
  const [exportOpen, setExportOpen] = useState(false)
  const scrollContainerRef = React.useRef<HTMLDivElement>(null)

  const { data: questionSet } = useQuestionSet(questionSetId || '', {
    enabled: !!questionSetId && !!executionId,
  })

  const { data: executionResults, isLoading: isLoadingResults } = useQuestionSetExecutionResults(
    questionSetId,
    executionId,
    undefined,
    { enabled: !!questionSetId && !!executionId }
  )

  useEffect(() => {
    if (window.location.hash && executionResults?.results && scrollContainerRef.current) {
      const hash = window.location.hash.substring(1)
      const element = document.getElementById(hash)
      if (element) {
        setTimeout(() => {
          element.scrollIntoView({ behavior: 'smooth', block: 'start' })
          setActiveSection(hash)
        }, 100)
      }
    }
  }, [executionResults])

  useEffect(() => {
    const scrollContainer = scrollContainerRef.current
    if (!scrollContainer) return

    const handleScroll = () => {
      if (!executionResults?.results) return
      
      const sections = executionResults.results.map((_, index) => ({
        id: `question-${index}`,
        element: document.getElementById(`question-${index}`),
      }))

      const containerRect = scrollContainer.getBoundingClientRect()
      const scrollTop = scrollContainer.scrollTop
      const containerTop = containerRect.top

      for (let i = sections.length - 1; i >= 0; i--) {
        const section = sections[i]
        if (section.element) {
          const elementRect = section.element.getBoundingClientRect()
          const elementTop = elementRect.top - containerTop + scrollTop
          
          if (elementTop <= scrollTop + 150) {
            setActiveSection(section.id)
            return
          }
        }
      }
      
      if (sections.length > 0) {
        setActiveSection(sections[0].id)
      }
    }

    scrollContainer.addEventListener('scroll', handleScroll)
    handleScroll()

    return () => scrollContainer.removeEventListener('scroll', handleScroll)
  }, [executionResults])

  const generateMarkdown = useMemo(() => {
    if (!executionResults?.results || executionResults.results.length === 0) {
      return ''
    }

    let markdown = `# Question Set Results\n\n`
    if (questionSet?.created) {
      markdown += `*Created on ${new Date(questionSet.created).toLocaleDateString()}*\n\n`
    }
    markdown += `---\n\n`

    executionResults.results.forEach((res, index) => {
      markdown += `## ${res.question}\n\n`
      if (res.response) {
        markdown += `${res.response}\n\n`
      }
      markdown += `---\n\n`
    })

    return markdown
  }, [executionResults, questionSet])

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
              gap: 0,
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
                  flexGrow: 1,
                }}
              >
                Question Set Results
              </Typography>
              <Box sx={{ display: 'flex', gap: 1 }}>
                {executionResults?.results && executionResults.results.length > 0 && (
                    <Tooltip title="Export to PDF">
                      <IconButton
                        color="secondary"
                        size="small"
                        onClick={() => setExportOpen(true)}
                      >
                        <img src={pdfIcon} alt="Export to PDF" style={{ width: 18, height: 18 }} />
                      </IconButton>
                    </Tooltip>
                )}
                {questionSetId && (
                  <Tooltip title="Edit Question Set">
                    <IconButton
                      color="secondary"
                      size="small"
                      onClick={() => {
                        account.orgNavigate('qa', {}, { questionSetId })
                      }}
                    >
                      <Edit size={18} />
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
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
            flexDirection: 'row',
            minHeight: 0,
            overflow: 'hidden',
            position: 'relative',
          }}
        >
          <Box
            ref={scrollContainerRef}
            sx={{
              flexGrow: 1,
              overflowY: 'auto',
              pr: 3,
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
                    const anchorId = `question-${index}`
                    return (
                      <Box
                        key={res.question_id || index}
                        id={anchorId}
                        sx={{
                          width: '100%',
                          p: 3,
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 2,
                          scrollMarginTop: '100px',
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
                                onClick={() => {
                                  let sessionUrl = `/session/${sessionId}`
                                  const org = account.organizationTools.organization
                                  if (org) {
                                    sessionUrl = `/org/${org.name}${sessionUrl}`
                                  }
                                  window.open(sessionUrl, '_blank')
                                }}
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

          {executionResults?.results && executionResults.results.length > 0 && (
            <Box
              sx={{
                width: 280,
                flexShrink: 0,
                display: { xs: 'none', lg: 'block' },
                position: 'sticky',
                top: 0,
                alignSelf: 'flex-start',
                height: 'fit-content',
                maxHeight: '100vh',
                overflowY: 'auto',
                pr: 3,
                pl: 2,
                py: 2,
                ...lightTheme.scrollbar,
              }}
            >
              <Typography
                variant="subtitle2"
                sx={{
                  fontWeight: 600,
                  mb: 2,
                  color: 'text.secondary',
                  textTransform: 'uppercase',
                  fontSize: '0.75rem',
                  letterSpacing: '0.08em',
                }}
              >
                Questions
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 1,
                }}
              >
                {executionResults.results.map((res, index) => {
                  const anchorId = `question-${index}`
                  const isActive = activeSection === anchorId
                  const truncatedQuestion = res.question.length > 60 
                    ? res.question.substring(0, 60) + '...' 
                    : res.question
                  
                  return (
                    <Link
                      key={index}
                      href={`#${anchorId}`}
                      onClick={(e) => {
                        e.preventDefault()
                        const element = document.getElementById(anchorId)
                        if (element) {
                          element.scrollIntoView({ behavior: 'smooth', block: 'start' })
                          setActiveSection(anchorId)
                          window.history.pushState(null, '', `#${anchorId}`)
                        }
                      }}
                      sx={{
                        fontSize: '0.875rem',
                        textDecoration: 'none',
                        color: isActive ? 'primary.main' : 'text.secondary',
                        borderLeft: isActive ? '2px solid' : '2px solid transparent',
                        borderColor: isActive ? 'primary.main' : 'transparent',
                        pl: 1.5,
                        py: 0.5,
                        transition: 'all 0.2s ease',
                        cursor: 'pointer',
                        '&:hover': {
                          color: 'primary.main',
                          borderColor: 'primary.main',
                        },
                        display: 'block',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      <Tooltip title={res.question} placement="left">
                        <span>{truncatedQuestion}</span>
                      </Tooltip>
                    </Link>
                  )
                })}
              </Box>
            </Box>
          )}
        </Box>
      </Box>

      <ExportDocument open={exportOpen} onClose={() => setExportOpen(false)}>
        <ToPDF
          markdown={generateMarkdown}
          filename={`question-set-results-${executionId}.pdf`}
          onClose={() => setExportOpen(false)}
        />
      </ExportDocument>
    </Box>
  )
}

export default QuestionSetResults

