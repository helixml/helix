import React, { FC, useState, useMemo, ComponentPropsWithoutRef } from 'react'
import ReactMarkdown, { Components } from 'react-markdown'
import { useQueries } from '@tanstack/react-query'
import {
  Box,
  Typography,
  Alert,
  Chip,
  Stack,
  Grid,
  Card,
  CardHeader,
  CardContent,
  Avatar,
  Drawer,
  IconButton,
  CircularProgress,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  TextField,
  InputAdornment,
  Paper,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material'
import {
  Brain,
  FileText,
  Code as CodeIcon,
  X as CloseIcon,
  Search as SearchIcon,
  GitBranch,
  Database,
  ChevronDown,
  Key,
  Plug,
  Copy,
  Eye,
  EyeOff,
} from 'lucide-react'
import MermaidDiagram, { extractMermaidDiagrams, hasMermaidDiagram } from '../widgets/MermaidDiagram'

import {
  useKoditEnrichmentDetail,
  useKoditCommits,
  useKoditStatus,
  useKoditSearch,
  groupEnrichmentsByType,
  getEnrichmentTypeName,
  getEnrichmentSubtypeName,
  koditEnrichmentDetailQueryKey,
  KODIT_TYPE_USAGE,
  KODIT_TYPE_DEVELOPER,
  KODIT_TYPE_LIVING_DOCUMENTATION,
  KODIT_SUBTYPE_ARCHITECTURE,
  KODIT_SUBTYPE_PHYSICAL,
  KODIT_SUBTYPE_DATABASE_SCHEMA,
} from '../../services/koditService'
import { useRouter } from '../../hooks/useRouter'
import useDebounce from '../../hooks/useDebounce'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetUserAPIKeys } from '../../services/userService'
import KoditStatusPill from './KoditStatusPill'

interface CodeIntelligenceTabProps {
  repository: any
  enrichments: any[]
  repoId: string
  commitSha?: string
}

const CodeIntelligenceTab: FC<CodeIntelligenceTabProps> = ({ repository, enrichments, repoId, commitSha }) => {
  const router = useRouter()
  const api = useApi()
  const apiClient = api.getApiClient()
  const snackbar = useSnackbar()
  const groupedEnrichmentsByType = groupEnrichmentsByType(enrichments)
  const { data: koditStatusData, isLoading: koditStatusLoading, error: koditStatusError } = useKoditStatus(repoId, { enabled: repoId && repository.kodit_indexing })

  // Get user's API keys for MCP connection
  const { data: apiKeys = [] } = useGetUserAPIKeys()
  const userApiKey = apiKeys?.[0]?.key || ''
  const [showApiKey, setShowApiKey] = useState(false)

  // Get the base URL for MCP endpoint
  const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''

  // Subtypes that may contain Mermaid diagrams (prioritize physical first)
  const DIAGRAM_SUBTYPES = [
    KODIT_SUBTYPE_PHYSICAL,
    KODIT_SUBTYPE_ARCHITECTURE,
    KODIT_SUBTYPE_DATABASE_SCHEMA,
  ]

  // Find enrichments that might contain Mermaid diagrams, sorted by priority
  const diagramEnrichmentIds = useMemo(() => {
    // Sort enrichments by subtype priority (physical first)
    const sorted = [...enrichments].sort((a, b) => {
      const aSubtype = a?.attributes?.subtype || ''
      const bSubtype = b?.attributes?.subtype || ''
      const aIndex = DIAGRAM_SUBTYPES.indexOf(aSubtype)
      const bIndex = DIAGRAM_SUBTYPES.indexOf(bSubtype)
      // If both are in DIAGRAM_SUBTYPES, sort by index; otherwise, non-diagram subtypes go last
      if (aIndex >= 0 && bIndex >= 0) return aIndex - bIndex
      if (aIndex >= 0) return -1
      if (bIndex >= 0) return 1
      return 0
    })

    const filtered = sorted.filter(e => {
      const subtype = e?.attributes?.subtype
      return DIAGRAM_SUBTYPES.includes(subtype)
    })
    return filtered.map(e => e.id).filter(Boolean)
  }, [enrichments])

  // Fetch full details for diagram enrichments to get complete content
  const diagramEnrichmentQueries = useQueries({
    queries: diagramEnrichmentIds.map(enrichmentId => ({
      queryKey: koditEnrichmentDetailQueryKey(repoId, enrichmentId),
      queryFn: async () => {
        const response = await apiClient.v1GitRepositoriesEnrichmentsDetail2(repoId, enrichmentId)
        return response.data
      },
      enabled: !!repoId && !!enrichmentId && repository.kodit_indexing,
      staleTime: 5 * 60 * 1000,
    })),
  })

  // Extract Mermaid diagrams from the full enrichment content
  const mermaidEnrichments = useMemo(() => {
    const diagramsWithSource: Array<{ diagram: string; enrichment: any; type: 'erd' | 'graph' }> = []

    for (const query of diagramEnrichmentQueries) {
      if (query.data?.attributes) {
        const enrichment = query.data
        const content = enrichment?.attributes?.content || ''

        if (hasMermaidDiagram(content)) {
          const diagrams = extractMermaidDiagrams(content)
          for (const diagram of diagrams) {
            const isERD = diagram.toLowerCase().includes('erdiagram')
            diagramsWithSource.push({
              diagram,
              enrichment,
              type: isERD ? 'erd' : 'graph',
            })
          }
        }
      }
    }

    return diagramsWithSource
  }, [diagramEnrichmentQueries])

  // Check if diagrams are still loading
  const diagramsLoading = diagramEnrichmentQueries.some(q => q.isLoading)

  // Fetch commits for the dropdown
  const { data: commits = [] } = useKoditCommits(repoId, 50, { enabled: repoId && repository.kodit_indexing })

  // Copy to clipboard helper
  const handleCopy = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    snackbar.success(`${label} copied to clipboard`)
  }

  // Custom markdown components to render mermaid diagrams inline
  const markdownComponents: Components = {
    code: ({ className, children, ...props }) => {
      const match = /language-(\w+)/.exec(className || '')
      const language = match ? match[1] : ''
      const codeContent = String(children).replace(/\n$/, '')

      if (language === 'mermaid') {
        return <MermaidDiagram code={codeContent} />
      }

      // Regular code block
      return (
        <Box
          component="pre"
          sx={{
            backgroundColor: 'rgba(0, 0, 0, 0.1)',
            padding: '1em',
            borderRadius: '4px',
            overflow: 'auto',
            fontSize: '0.85em',
            fontFamily: 'monospace',
          }}
        >
          <code className={className} {...props}>
            {children}
          </code>
        </Box>
      )
    },
  }

  const [selectedEnrichmentId, setSelectedEnrichmentId] = useState<string | null>(null)
  const enrichmentDrawerOpen = !!selectedEnrichmentId

  const { data: enrichmentDetail, isLoading: enrichmentDetailLoading } = useKoditEnrichmentDetail(
    repoId,
    selectedEnrichmentId || '',
    { enabled: enrichmentDrawerOpen }
  )

  // Search state
  const [searchQuery, setSearchQuery] = useState('')
  const debouncedSearchQuery = useDebounce(searchQuery, 300)
  const [selectedSnippet, setSelectedSnippet] = useState<any>(null)

  // Search snippets
  const { data: searchResults = [], isLoading: searchLoading } = useKoditSearch(
    repoId,
    debouncedSearchQuery,
    20,
    commitSha,
    { enabled: repoId && repository.kodit_indexing && debouncedSearchQuery.trim().length > 0 }
  )

  const handleCommitChange = (newCommitSha: string) => {
    if (newCommitSha === 'all') {
      router.removeParams(['commit'])
    } else {
      router.mergeParams({ commit: newCommitSha })
    }
  }

  const handleClearSearch = () => {
    setSearchQuery('')
  }

  // Global link styles for the entire Code Intelligence section
  const globalLinkStyles = {
    '& a': {
      color: '#00d5ff',
      textDecoration: 'none',
      '&:hover': {
        textDecoration: 'underline',
      },
      '&:visited': {
        color: '#00d5ff',
      },
    },
  }

  return (
    <>
      <Box sx={{ maxWidth: 1200, ...globalLinkStyles }}>
        {repository.kodit_indexing ? (
          <Box sx={{ mb: 4 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3, flexWrap: 'wrap' }}>
              <Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                  <Brain size={24} />
                  <Typography variant="h5" sx={{ fontWeight: 600 }}>
                    Code Intelligence
                  </Typography>
                  <KoditStatusPill
                    data={koditStatusData}
                    isLoading={koditStatusLoading}
                    error={koditStatusError}
                  />
                </Box>
                <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5, ml: 4.5 }}>
                  Powered by Kodit. Available via the built-in MCP server for Helix code agents.
                </Typography>
              </Box>

              {commits.length > 0 && (
                <FormControl size="small" sx={{ minWidth: 300 }}>
                  <InputLabel>Commit</InputLabel>
                  <Select
                    value={commitSha || 'all'}
                    label="Commit"
                    onChange={(e) => handleCommitChange(e.target.value)}
                  >
                    <MenuItem value="all">Latest Commit</MenuItem>
                    {commits.map((commit: any) => (
                      <MenuItem key={commit.id} value={commit.id}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                          <Box
                            component="span"
                            sx={{ fontFamily: 'monospace', fontSize: '0.85rem', color: 'text.secondary' }}
                          >
                            {commit.id?.substring(0, 7)}
                          </Box>
                          <Box
                            component="span"
                            sx={{
                              flex: 1,
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                            }}
                          >
                            {commit.attributes?.message || 'No message'}
                          </Box>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}

              <TextField
                size="small"
                placeholder="Search code snippets..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                sx={{ minWidth: 300 }}
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon size={18} />
                    </InputAdornment>
                  ),
                  endAdornment: searchQuery && (
                    <InputAdornment position="end">
                      <IconButton size="small" onClick={handleClearSearch} edge="end">
                        <CloseIcon size={16} />
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>

            {koditStatusData?.message && (
              <Alert severity="info" sx={{ mb: 3 }}>
                {koditStatusData.message}
              </Alert>
            )}

            {/* MCP Client Connection Instructions - hide when searching */}
            {!debouncedSearchQuery && (
            <Accordion
              sx={{
                mb: 3,
                bgcolor: 'rgba(0, 213, 255, 0.04)',
                border: '1px solid',
                borderColor: 'rgba(0, 213, 255, 0.2)',
                borderRadius: '8px !important',
                '&:before': { display: 'none' },
                '&.Mui-expanded': { margin: '0 0 24px 0' },
              }}
            >
              <AccordionSummary
                expandIcon={<ChevronDown size={20} color="#00d5ff" />}
                sx={{
                  '& .MuiAccordionSummary-content': { alignItems: 'center', gap: 1 },
                }}
              >
                <Plug size={18} color="#00d5ff" />
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: '#00d5ff' }}>
                  Connect External MCP Clients
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  Connect your local coding agents (Claude Code, Cursor, Cline, Roo Code, Codex, Gemini CLI, Qwen Code, Zed, etc.) to access this repository's code intelligence via the Kodit MCP server.
                </Typography>

                <Stack spacing={2}>
                  <Box>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                      MCP Endpoint URL
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <TextField
                        size="small"
                        fullWidth
                        value={`${baseUrl}/api/v1/mcp/kodit`}
                        InputProps={{
                          readOnly: true,
                          sx: { fontFamily: 'monospace', fontSize: '0.85rem', bgcolor: 'rgba(0, 0, 0, 0.1)' },
                        }}
                      />
                      <IconButton
                        size="small"
                        onClick={() => handleCopy(`${baseUrl}/api/v1/mcp/kodit`, 'MCP endpoint URL')}
                        sx={{ color: '#00d5ff' }}
                      >
                        <Copy size={16} />
                      </IconButton>
                    </Box>
                  </Box>

                  <Box>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                      Authentication
                    </Typography>
                    {userApiKey ? (
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <TextField
                          size="small"
                          fullWidth
                          type={showApiKey ? 'text' : 'password'}
                          value={userApiKey}
                          InputProps={{
                            readOnly: true,
                            sx: { fontFamily: 'monospace', fontSize: '0.85rem', bgcolor: 'rgba(0, 0, 0, 0.1)' },
                            startAdornment: (
                              <Box sx={{ display: 'flex', alignItems: 'center', mr: 1 }}>
                                <Key size={16} color="#00d5ff" />
                              </Box>
                            ),
                          }}
                        />
                        <IconButton
                          size="small"
                          onClick={() => setShowApiKey(!showApiKey)}
                          sx={{ color: '#00d5ff' }}
                        >
                          {showApiKey ? <EyeOff size={16} /> : <Eye size={16} />}
                        </IconButton>
                        <IconButton
                          size="small"
                          onClick={() => handleCopy(userApiKey, 'API key')}
                          sx={{ color: '#00d5ff' }}
                        >
                          <Copy size={16} />
                        </IconButton>
                      </Box>
                    ) : (
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 1.5, bgcolor: 'rgba(0, 0, 0, 0.1)', borderRadius: 1 }}>
                        <Key size={16} color="#00d5ff" />
                        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                          Use your Helix API key in the <code>Authorization: Bearer &lt;api_key&gt;</code> header
                        </Typography>
                      </Box>
                    )}
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                      Add as <code>Authorization: Bearer &lt;api_key&gt;</code> header
                    </Typography>
                  </Box>

                  <Alert severity="info" sx={{ fontSize: '0.8rem' }}>
                    <Typography variant="body2">
                      <strong>Quick Setup:</strong> In your MCP client configuration, add this server with the URL above and your API key.
                      The Kodit MCP server provides tools for semantic code search, architecture diagrams, and documentation.
                    </Typography>
                  </Alert>
                </Stack>
              </AccordionDetails>
            </Accordion>
            )}

            {/* Prominent Mermaid Diagrams Section - hide when searching */}
            {!debouncedSearchQuery && (
            <>
            {diagramsLoading && diagramEnrichmentIds.length > 0 && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
                <CircularProgress size={20} />
                <Typography variant="body2" color="text.secondary">
                  Loading diagrams...
                </Typography>
              </Box>
            )}
            {mermaidEnrichments.length > 0 && (
              <Box sx={{ mb: 4 }}>
                <Grid container spacing={3}>
                  {mermaidEnrichments.map((item, index) => (
                    <Grid item xs={12} md={6} key={`mermaid-${index}`}>
                      <Paper
                        elevation={0}
                        sx={{
                          p: 3,
                          borderRadius: 3,
                          background: 'linear-gradient(135deg, rgba(0, 213, 255, 0.08) 0%, rgba(0, 213, 255, 0.02) 100%)',
                          border: '1px solid',
                          borderColor: 'rgba(0, 213, 255, 0.3)',
                          transition: 'all 0.3s ease-in-out',
                          '&:hover': {
                            borderColor: 'rgba(0, 213, 255, 0.6)',
                            boxShadow: '0 8px 32px rgba(0, 213, 255, 0.15)',
                          },
                        }}
                      >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                          {item.type === 'erd' ? (
                            <Database size={20} color="#00d5ff" />
                          ) : (
                            <GitBranch size={20} color="#00d5ff" />
                          )}
                          <Typography variant="h6" sx={{ fontWeight: 600, color: '#00d5ff' }}>
                            {item.type === 'erd' ? 'Database Schema' : 'Architecture Diagram'}
                          </Typography>
                          <Chip
                            label={item.enrichment?.attributes?.subtype || 'architecture'}
                            size="small"
                            sx={{
                              bgcolor: 'rgba(0, 213, 255, 0.15)',
                              color: '#00d5ff',
                              fontWeight: 600,
                              fontSize: '0.7rem',
                            }}
                          />
                        </Box>
                        <MermaidDiagram code={item.diagram} />
                      </Paper>
                    </Grid>
                  ))}
                </Grid>
              </Box>
            )}
            </>
            )}
          </Box>
        ) : (
          <Alert severity="info" sx={{ mb: 4 }}>
            Code Intelligence is not enabled for this repository. Enable it in the Settings tab to start indexing.
          </Alert>
        )}

        {debouncedSearchQuery && (
          <Box sx={{ mb: 4 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
              <Typography variant="h5" sx={{ fontWeight: 600 }}>
                Search Results
              </Typography>
              {searchLoading ? (
                <CircularProgress size={20} />
              ) : (
                <Chip label={`${searchResults.length} result${searchResults.length !== 1 ? 's' : ''}`} size="small" />
              )}
            </Box>

            {searchResults.length > 0 ? (
              <Grid container spacing={2}>
                {searchResults.map((snippet: any, index: number) => {
                  // Use the properly typed fields from KoditSearchResult
                  const snippetId = snippet.id || `snippet-${index}`
                  const snippetType = snippet.type || 'snippet'
                  const language = snippet.language || 'unknown'
                  const content = snippet.content || 'No content available'
                  const filePath = snippet.file_path || ''

                  // Use file path as title if available, otherwise use snippet ID
                  let fileName = null
                  if (filePath) {
                    // Extract filename from path
                    fileName = filePath.split('/').pop() || filePath
                  }

                  return (
                    <Grid item xs={12} sm={6} md={4} lg={3} key={snippetId}>
                      <Card
                        onClick={() => {
                          setSelectedSnippet(snippet)
                          setSelectedEnrichmentId(null)
                        }}
                        sx={{
                          height: 280,
                          display: 'flex',
                          flexDirection: 'column',
                          boxShadow: 1,
                          borderStyle: 'dashed',
                          borderWidth: 1,
                          borderColor: 'warning.main',
                          cursor: 'pointer',
                          transition: 'all 0.2s',
                          '&:hover': {
                            boxShadow: 4,
                            transform: 'translateY(-4px)',
                            borderColor: 'warning.main',
                            borderWidth: 2,
                            borderStyle: 'solid',
                          },
                        }}
                      >
                        <CardHeader
                          avatar={
                            <Avatar sx={{ bgcolor: 'white', width: 40, height: 40, border: '2px solid', borderColor: 'warning.main' }}>
                              <SearchIcon size={24} color="#ed6c02" />
                            </Avatar>
                          }
                          title={snippetType}
                          titleTypographyProps={{ variant: 'subtitle1', fontWeight: 600, fontSize: '0.95rem' }}
                          subheader={fileName}
                          subheaderTypographyProps={{ variant: 'caption', fontSize: '0.7rem' }}
                          sx={{ pb: 1 }}
                        />
                        <CardContent sx={{
                          flexGrow: 1,
                          pt: 0,
                          overflow: 'hidden',
                          display: 'flex',
                          flexDirection: 'column',
                        }}>
                          <Box
                            component="pre"
                            sx={{
                              fontSize: '0.75rem',
                              lineHeight: 1.4,
                              overflow: 'hidden',
                              color: 'text.secondary',
                              display: '-webkit-box',
                              WebkitLineClamp: 9,
                              WebkitBoxOrient: 'vertical',
                              fontFamily: 'monospace',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-word',
                              margin: 0,
                            }}
                          >
                            {content}
                          </Box>
                        </CardContent>
                      </Card>
                    </Grid>
                  )
                })}
              </Grid>
            ) : searchLoading ? (
              <Box sx={{ textAlign: 'center', py: 4 }}>
                <CircularProgress />
                <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
                  Searching...
                </Typography>
              </Box>
            ) : (
              <Alert severity="info">
                No snippets found for &quot;{debouncedSearchQuery}&quot;
              </Alert>
            )}
          </Box>
        )}

        {enrichments.length > 0 && Object.keys(groupedEnrichmentsByType).length > 0 ? (
          <Stack spacing={4}>
            {[KODIT_TYPE_DEVELOPER, KODIT_TYPE_USAGE, KODIT_TYPE_LIVING_DOCUMENTATION, ...Object.keys(groupedEnrichmentsByType).filter(t =>
              t !== KODIT_TYPE_DEVELOPER && t !== KODIT_TYPE_USAGE && t !== KODIT_TYPE_LIVING_DOCUMENTATION
            )].map((type) => {
              const typeEnrichments = groupedEnrichmentsByType[type]
              if (!typeEnrichments || typeEnrichments.length === 0) return null

              const typeName = getEnrichmentTypeName(type)
              const typeDescription = type === KODIT_TYPE_DEVELOPER
                ? 'Architecture, APIs, and technical documentation'
                : type === KODIT_TYPE_USAGE
                  ? 'How-to guides and usage examples'
                  : 'Recent changes and commit descriptions'

              return (
                <Box key={type}>
                  <Box sx={{ mb: 3 }}>
                    <Typography variant="h5" sx={{ fontWeight: 600, mb: 0.5 }}>
                      {typeName}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {typeDescription}
                    </Typography>
                  </Box>

                  <Grid container spacing={2}>
                    {typeEnrichments.map((enrichment: any, index: number) => {
                      const subtype = enrichment.attributes?.subtype
                      const subtypeName = getEnrichmentSubtypeName(subtype)

                      const borderColor = type === KODIT_TYPE_DEVELOPER
                        ? 'primary.main'
                        : type === KODIT_TYPE_USAGE
                          ? 'success.main'
                          : 'info.main'
                      const iconColor = type === KODIT_TYPE_DEVELOPER
                        ? '#1976d2'
                        : type === KODIT_TYPE_USAGE
                          ? '#2e7d32'
                          : '#0288d1'

                      return (
                        <Grid item xs={12} sm={6} md={4} lg={3} key={`${type}-${subtype}-${enrichment.id || index}`}>
                          <Card
                            onClick={() => {
                              if (enrichment.id) {
                                setSelectedEnrichmentId(enrichment.id)
                              }
                            }}
                            sx={{
                              height: 280,
                              display: 'flex',
                              flexDirection: 'column',
                              boxShadow: 1,
                              borderStyle: 'dashed',
                              borderWidth: 1,
                              borderColor: 'divider',
                              cursor: 'pointer',
                              transition: 'all 0.2s',
                              '&:hover': {
                                boxShadow: 4,
                                transform: 'translateY(-4px)',
                                borderColor: borderColor,
                                borderWidth: 2,
                                borderStyle: 'solid',
                              },
                            }}
                          >
                            <CardHeader
                              avatar={
                                <Avatar sx={{ bgcolor: 'white', width: 40, height: 40, border: '2px solid', borderColor: borderColor }}>
                                  {type === KODIT_TYPE_DEVELOPER ? (
                                    <Brain size={24} color={iconColor} />
                                  ) : type === KODIT_TYPE_USAGE ? (
                                    <FileText size={24} color={iconColor} />
                                  ) : (
                                    <CodeIcon size={24} color={iconColor} />
                                  )}
                                </Avatar>
                              }
                              title={subtypeName}
                              titleTypographyProps={{ variant: 'subtitle1', fontWeight: 600, fontSize: '0.95rem' }}
                              subheader={enrichment.attributes?.updated_at ? new Date(enrichment.attributes.updated_at).toLocaleDateString() : ''}
                              subheaderTypographyProps={{ variant: 'caption', fontSize: '0.7rem' }}
                              sx={{ pb: 1 }}
                            />
                            <CardContent sx={{
                              flexGrow: 1,
                              pt: 0,
                              overflow: 'hidden',
                              display: 'flex',
                              flexDirection: 'column',
                            }}>
                              <Box
                                sx={{
                                  fontSize: '0.8rem',
                                  lineHeight: 1.5,
                                  overflow: 'hidden',
                                  color: 'text.secondary',
                                  display: '-webkit-box',
                                  WebkitLineClamp: 8,
                                  WebkitBoxOrient: 'vertical',
                                  '& p': {
                                    margin: '0 0 0.5em 0',
                                    '&:last-child': { margin: 0 }
                                  },
                                  '& ul, & ol': {
                                    margin: '0 0 0.5em 0',
                                    paddingLeft: '1.2em'
                                  },
                                  '& li': {
                                    margin: '0.2em 0'
                                  },
                                  '& code': {
                                    backgroundColor: 'rgba(0, 0, 0, 0.05)',
                                    padding: '0.1em 0.3em',
                                    borderRadius: '3px',
                                    fontSize: '0.9em',
                                    fontFamily: 'monospace'
                                  },
                                  '& pre': {
                                    backgroundColor: 'rgba(0, 0, 0, 0.05)',
                                    padding: '0.5em',
                                    borderRadius: '4px',
                                    overflow: 'auto',
                                    fontSize: '0.85em'
                                  },
                                  '& h1, & h2, & h3, & h4, & h5, & h6': {
                                    margin: '0.5em 0 0.3em 0',
                                    fontWeight: 600
                                  },
                                  '& a': {
                                    color: '#00d5ff',
                                    textDecoration: 'none',
                                    '&:hover': {
                                      textDecoration: 'underline'
                                    }
                                  }
                                }}
                              >
                                <ReactMarkdown>
                                  {enrichment.attributes?.content || 'No content available'}
                                </ReactMarkdown>
                              </Box>
                            </CardContent>
                          </Card>
                        </Grid>
                      )
                    })}
                  </Grid>
                </Box>
              )
            })}
          </Stack>
        ) : repository.kodit_indexing ? (
          <Box sx={{ textAlign: 'center', py: 8 }}>
            <Brain size={48} color="#656d76" style={{ marginBottom: 16, opacity: 0.5 }} />
            <Typography variant="h6" color="text.secondary" gutterBottom>
              {commitSha ? 'No enrichments for this commit' : 'No enrichments available yet'}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {commitSha
                ? 'This commit does not have any code intelligence enrichments.'
                : 'Code Intelligence is indexing your repository. Check back soon.'}
            </Typography>
          </Box>
        ) : null}
      </Box>

      <Drawer
        anchor="right"
        open={enrichmentDrawerOpen || !!selectedSnippet}
        onClose={() => {
          setSelectedEnrichmentId(null)
          setSelectedSnippet(null)
        }}
        sx={{
          '& .MuiDrawer-paper': {
            width: { xs: '100%', sm: '600px', md: '700px' },
            p: 3,
          },
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 3 }}>
          <Box>
            <Typography variant="h5" gutterBottom>
              {selectedSnippet ? 'Code Snippet' : 'Enrichment Details'}
            </Typography>
            {selectedSnippet && (
              <Typography variant="caption" color="text.secondary" display="block">
                {selectedSnippet.file_path ? (
                  selectedSnippet.file_path.split('/').pop()
                ) : (
                  selectedSnippet.id
                )}
              </Typography>
            )}
            {enrichmentDetail && !selectedSnippet && (
              <Typography variant="caption" color="text.secondary" display="block">
                {getEnrichmentSubtypeName(enrichmentDetail.attributes?.subtype || '')}
              </Typography>
            )}
          </Box>
          <IconButton
            onClick={() => {
              setSelectedEnrichmentId(null)
              setSelectedSnippet(null)
            }}
            size="small"
          >
            <CloseIcon size={20} />
          </IconButton>
        </Box>

        {selectedSnippet ? (
          <Box>
            <Stack direction="row" spacing={1} sx={{ mb: 3, flexWrap: 'wrap', gap: 1 }}>
              <Chip label={selectedSnippet.language || 'unknown'} size="small" color="warning" />
              <Chip label={selectedSnippet.type || 'snippet'} size="small" variant="outlined" />
              {selectedSnippet.file_path && (
                <Chip label={selectedSnippet.file_path} size="small" variant="outlined" />
              )}
            </Stack>

            <Box
              component="pre"
              sx={{
                fontFamily: 'monospace',
                fontSize: '0.875rem',
                lineHeight: 1.5,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                backgroundColor: 'rgba(0, 0, 0, 0.05)',
                padding: '1em',
                borderRadius: '4px',
                overflow: 'auto',
                margin: 0,
              }}
            >
              {selectedSnippet.content || 'No content available'}
            </Box>
          </Box>
        ) : enrichmentDetailLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '200px' }}>
            <CircularProgress />
          </Box>
        ) : enrichmentDetail ? (
          <Box>
            <Stack direction="row" spacing={1} sx={{ mb: 3, flexWrap: 'wrap', gap: 1 }}>
              <Chip
                label={getEnrichmentTypeName(enrichmentDetail.attributes?.type || '')}
                size="small"
                color={
                  enrichmentDetail.attributes?.type === KODIT_TYPE_DEVELOPER
                    ? 'primary'
                    : enrichmentDetail.attributes?.type === KODIT_TYPE_USAGE
                      ? 'success'
                      : 'info'
                }
              />
              {enrichmentDetail.attributes?.updated_at && (
                <Chip
                  label={`Updated: ${new Date(enrichmentDetail.attributes.updated_at).toLocaleDateString()}`}
                  size="small"
                  variant="outlined"
                />
              )}
            </Stack>

            {/* Render content with mermaid diagrams inline */}
            <Box
              sx={{
                '& p': {
                  margin: '0 0 1em 0',
                  '&:last-child': { margin: 0 },
                },
                '& ul, & ol': {
                  margin: '0 0 1em 0',
                  paddingLeft: '1.5em',
                },
                '& li': {
                  margin: '0.5em 0',
                },
                '& code': {
                  backgroundColor: 'rgba(0, 0, 0, 0.05)',
                  padding: '0.2em 0.4em',
                  borderRadius: '3px',
                  fontSize: '0.9em',
                  fontFamily: 'monospace',
                },
                '& h1, & h2, & h3, & h4, & h5, & h6': {
                  margin: '1em 0 0.5em 0',
                  fontWeight: 600,
                  '&:first-child': {
                    marginTop: 0,
                  },
                },
                '& a': {
                  color: '#00d5ff',
                  textDecoration: 'none',
                  '&:hover': {
                    textDecoration: 'underline'
                  }
                },
              }}
            >
              <ReactMarkdown components={markdownComponents}>
                {enrichmentDetail.attributes?.content || ''}
              </ReactMarkdown>
            </Box>
          </Box>
        ) : (
          <Alert severity="error">Failed to load enrichment details</Alert>
        )}
      </Drawer>
    </>
  )
}

export default CodeIntelligenceTab
