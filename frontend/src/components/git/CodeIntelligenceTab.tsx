import React, { FC, useState } from 'react'
import ReactMarkdown from 'react-markdown'
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
} from '@mui/material'
import {
  Brain,
  FileText,
  Code as CodeIcon,
  X as CloseIcon,
} from 'lucide-react'

import {
  useKoditEnrichmentDetail,
  useKoditStatus,
  groupEnrichmentsByType,
  getEnrichmentTypeName,
  getEnrichmentSubtypeName,
  KODIT_TYPE_USAGE,
  KODIT_TYPE_DEVELOPER,
  KODIT_TYPE_LIVING_DOCUMENTATION,
} from '../../services/koditService'

interface CodeIntelligenceTabProps {
  repository: any
  enrichments: any[]
  repoId: string
}

const CodeIntelligenceTab: FC<CodeIntelligenceTabProps> = ({ repository, enrichments, repoId }) => {
  const groupedEnrichmentsByType = groupEnrichmentsByType(enrichments)
  const { data: koditStatusData } = useKoditStatus(repoId, { enabled: !!repoId && !!repository?.metadata?.kodit_indexing })

  const [selectedEnrichmentId, setSelectedEnrichmentId] = useState<string | null>(null)
  const enrichmentDrawerOpen = !!selectedEnrichmentId

  const { data: enrichmentDetail, isLoading: enrichmentDetailLoading } = useKoditEnrichmentDetail(
    repoId,
    selectedEnrichmentId || '',
    { enabled: enrichmentDrawerOpen }
  )

  return (
    <>
      <Box sx={{ maxWidth: 1200 }}>
        {repository.metadata?.kodit_indexing ? (
          <Box sx={{ mb: 4 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
              <Brain size={24} />
              <Typography variant="h5" sx={{ fontWeight: 600 }}>
                Code Intelligence
              </Typography>
              <Chip
                label={koditStatusData?.status || 'Active'}
                size="small"
                color={koditStatusData?.status === 'completed' ? 'success' : koditStatusData?.status === 'failed' ? 'error' : 'warning'}
                sx={{ ml: 1 }}
              />
            </Box>

            {koditStatusData?.message && (
              <Alert severity="info" sx={{ mb: 3 }}>
                {koditStatusData.message}
              </Alert>
            )}
          </Box>
        ) : (
          <Alert severity="info" sx={{ mb: 4 }}>
            Code Intelligence is not enabled for this repository. Enable it in the Settings tab to start indexing.
          </Alert>
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
        ) : repository.metadata?.kodit_indexing ? (
          <Box sx={{ textAlign: 'center', py: 8 }}>
            <Brain size={48} color="#656d76" style={{ marginBottom: 16, opacity: 0.5 }} />
            <Typography variant="h6" color="text.secondary" gutterBottom>
              No enrichments available yet
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Code Intelligence is indexing your repository. Check back soon.
            </Typography>
          </Box>
        ) : null}
      </Box>

      <Drawer
        anchor="right"
        open={enrichmentDrawerOpen}
        onClose={() => {
          setSelectedEnrichmentId(null)
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
              Enrichment Details
            </Typography>
            {enrichmentDetail && (
              <Typography variant="caption" color="text.secondary" display="block">
                {getEnrichmentSubtypeName(enrichmentDetail.attributes?.subtype || '')}
              </Typography>
            )}
          </Box>
          <IconButton
            onClick={() => {
              setSelectedEnrichmentId(null)
            }}
            size="small"
          >
            <CloseIcon size={20} />
          </IconButton>
        </Box>

        {enrichmentDetailLoading ? (
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
                '& pre': {
                  backgroundColor: 'rgba(0, 0, 0, 0.05)',
                  padding: '1em',
                  borderRadius: '4px',
                  overflow: 'auto',
                  fontSize: '0.85em',
                },
                '& h1, & h2, & h3, & h4, & h5, & h6': {
                  margin: '1em 0 0.5em 0',
                  fontWeight: 600,
                  '&:first-child': {
                    marginTop: 0,
                  },
                },
              }}
            >
              <ReactMarkdown>{enrichmentDetail.attributes?.content || 'No content available'}</ReactMarkdown>
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
