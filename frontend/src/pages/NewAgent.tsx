import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import LinkIcon from '@mui/icons-material/Link'
import TextFieldsIcon from '@mui/icons-material/TextFields'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useApps from '../hooks/useApps'
import { ICreateAgentParams } from '../contexts/apps'


const NewAgent: FC = () => {
  const account = useAccount()  
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const apps = useApps()

  const [name, setName] = useState('')
  const [systemPrompt, setSystemPrompt] = useState('')
  const [knowledgeType, setKnowledgeType] = useState<'url' | 'text' | 'file'>('url')
  const [knowledgeUrl, setKnowledgeUrl] = useState('')
  const [knowledgeText, setKnowledgeText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name || !systemPrompt) {
      snackbar.error('Please fill in all required fields')
      return
    }

    setIsSubmitting(true)
    try {
      // Create knowledge source if provided
      const knowledge = []
      if (knowledgeType === 'url' && knowledgeUrl) {
        knowledge.push({
          id: '', // This will be set by the server
          name: 'Web Knowledge',
          description: 'Knowledge from web URL',
          source: {
            web: {
              urls: [knowledgeUrl],
              crawler: {
                enabled: true,
                max_depth: 1,
                max_pages: 5,
                readability: true
              }
            }
          },
          refresh_schedule: '',
          version: '',
          state: 'pending',
          rag_settings: {
            results_count: 0,
            chunk_size: 0,
            chunk_overflow: 0,
            enable_vision: false,
          }
        })
      } else if (knowledgeType === 'text' && knowledgeText) {
        knowledge.push({          
          id: '', // This will be set by the server
          name: 'Text Knowledge',
          description: 'Knowledge from text input',
          source: {
            text: knowledgeText
          },
          refresh_schedule: '',
          version: '',
          state: 'pending',
          rag_settings: {
            results_count: 0,
            chunk_size: 0,
            chunk_overflow: 0,
            enable_vision: false,
          }
        })
      }

      // Create the agent with the new parameter structure
      const agentParams: ICreateAgentParams = {
        name,
        systemPrompt,
        knowledge: knowledge.length > 0 ? knowledge : undefined,
      }

      const newApp = await apps.createAgent(agentParams)

      if (!newApp) {
        throw new Error('Failed to create agent')
      }

      // Navigate to the agent pages (org aware)
      account.orgNavigate('app', { app_id: newApp.id })
      snackbar.success('Agent created successfully')
    } catch (error) {
      console.error('Error creating agent:', error)
      snackbar.error('Failed to create agent')
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!account.user) return null

  return (
    <Page
      showDrawerButton={false}
      orgBreadcrumbs={true}
      breadcrumbs={[
        {
          title: 'Agents',
          routeName: 'apps'
        },
        {
          title: 'New',
        }
      ]}
    >
      <Container maxWidth="md" sx={{ mt: 4 }}>
        <Paper 
          elevation={0}
          sx={{ 
            p: 4,
            backgroundColor: themeConfig.darkPanel,
            borderRadius: 2,
            boxShadow: '0 4px 24px 0 rgba(0,0,0,0.12)'
          }}
        >
          <Typography variant="h4" sx={{ mb: 4, fontWeight: 'bold' }}>
            Create New Agent
          </Typography>

          <form onSubmit={handleSubmit}>
            <Grid container spacing={3}>
              {/* Agent Name */}
              <Grid item xs={12}>
                <TextField
                  required
                  fullWidth
                  label="Agent Name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  helperText="Give your agent a descriptive name"
                  sx={{ mb: 2 }}
                />
              </Grid>

              {/* System Prompt */}
              <Grid item xs={12}>
                <TextField
                  required
                  fullWidth
                  multiline
                  rows={4}
                  label="System Prompt"
                  value={systemPrompt}
                  onChange={(e) => setSystemPrompt(e.target.value)}
                  helperText="Define how your agent should behave and what it should do"
                  sx={{ mb: 2 }}
                />
              </Grid>

              {/* Knowledge Section */}
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2 }}>
                  Add Knowledge (Optional)
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                  You can add knowledge to your agent by providing a URL, text, or uploading files.
                </Typography>

                <Box sx={{ mb: 3 }}>
                  <Button
                    variant={knowledgeType === 'url' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'url' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('url')}
                    startIcon={<LinkIcon />}
                    sx={{ mr: 2 }}
                  >
                    URL
                  </Button>
                  <Button
                    variant={knowledgeType === 'text' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'text' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('text')}
                    startIcon={<TextFieldsIcon />}
                    sx={{ mr: 2 }}
                  >
                    Text
                  </Button>
                  <Button
                    variant={knowledgeType === 'file' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'file' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('file')}
                    startIcon={<CloudUploadIcon />}
                  >
                    File
                  </Button>
                </Box>

                {knowledgeType === 'url' && (
                  <TextField
                    fullWidth
                    label="Knowledge URL"
                    value={knowledgeUrl}
                    onChange={(e) => setKnowledgeUrl(e.target.value)}
                    helperText="Enter a URL containing knowledge for your agent"
                  />
                )}

                {knowledgeType === 'text' && (
                  <TextField
                    fullWidth
                    multiline
                    rows={4}
                    label="Knowledge Text"
                    value={knowledgeText}
                    onChange={(e) => setKnowledgeText(e.target.value)}
                    helperText="Enter text containing knowledge for your agent"
                  />
                )}

                {knowledgeType === 'file' && (
                  <Box sx={{ textAlign: 'center', py: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                      File upload will be available after agent creation
                    </Typography>
                  </Box>
                )}
              </Grid>

              {/* Submit Button */}
              <Grid item xs={12}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                 Additional configuration like avatar, description, and skills can be set up after creation.
                </Typography>
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 2 }}>
                  <Button
                    type="submit"
                    variant="outlined"
                    color="secondary"
                    size="large"
                    disabled={isSubmitting}
                  >
                    Create Agent
                  </Button>
                </Box>
              </Grid>
            </Grid>
          </form>
        </Paper>
      </Container>
    </Page>
  )
}

export default NewAgent