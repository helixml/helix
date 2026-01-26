import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
  Stepper,
  Step,
  StepLabel,
  CircularProgress,
  Alert,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Chip,
  RadioGroup,
  Radio,
  FormControlLabel,
  Divider,
} from '@mui/material'
import {
  CheckCircle,
  XCircle,
  GitFork,
  Github,
  Lock,
  Unlock,
} from 'lucide-react'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import {
  TypesCheckSampleProjectAccessResponse,
  TypesRepositoryAccessCheck,
  ServerSampleProject,
} from '../../api/api'

interface SampleProjectWizardProps {
  open: boolean
  onClose: () => void
  onComplete: (projectId: string) => void
  sampleProject: ServerSampleProject | null
  organizationId?: string
  selectedAgentId?: string
}

type Step = 'github-check' | 'access-check' | 'fork-decision' | 'creating'

const SampleProjectWizard: FC<SampleProjectWizardProps> = ({
  open,
  onClose,
  onComplete,
  sampleProject,
  organizationId,
  selectedAgentId,
}) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const snackbar = useSnackbar()

  // State
  const [currentStep, setCurrentStep] = useState<Step>('github-check')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [accessInfo, setAccessInfo] = useState<TypesCheckSampleProjectAccessResponse | null>(null)
  const [forkDecision, setForkDecision] = useState<'use_original' | 'fork'>('use_original')
  const [repoDecisions, setRepoDecisions] = useState<Record<string, string>>({})
  const [gitHubConnections, setGitHubConnections] = useState<any[]>([])
  const [selectedConnectionId, setSelectedConnectionId] = useState<string>('')

  // Reset state when dialog opens
  useEffect(() => {
    if (open && sampleProject) {
      setCurrentStep('github-check')
      setIsLoading(true)
      setError(null)
      setAccessInfo(null)
      setRepoDecisions({})
      checkGitHubConnection()
    }
  }, [open, sampleProject?.id])

  // Check for GitHub OAuth connection
  const checkGitHubConnection = async () => {
    try {
      // Fetch user's OAuth connections
      const connectionsResp = await apiClient.v1OauthConnectionsList()
      const connections = connectionsResp.data || []

      // Filter for GitHub connections
      const ghConnections = connections.filter(
        (conn: any) => conn.provider?.type === 'github'
      )
      setGitHubConnections(ghConnections)

      if (ghConnections.length > 0) {
        setSelectedConnectionId(ghConnections[0].id)
        // Check repo access
        await checkRepoAccess(ghConnections[0].id)
      } else {
        setIsLoading(false)
        setCurrentStep('github-check')
      }
    } catch (err: any) {
      console.error('Failed to check GitHub connection:', err)
      setIsLoading(false)
      setError('Failed to check GitHub connection')
    }
  }

  // Check repository access
  const checkRepoAccess = async (connectionId: string) => {
    if (!sampleProject) return

    try {
      setIsLoading(true)
      const response = await apiClient.v1SampleProjectsSimpleCheckAccessCreate({
        sample_project_id: sampleProject.id || '',
        github_connection_id: connectionId,
      })

      setAccessInfo(response.data)

      // Initialize repo decisions
      const decisions: Record<string, string> = {}
      response.data.repositories?.forEach((repo: TypesRepositoryAccessCheck) => {
        decisions[repo.github_url || ''] = repo.has_write_access ? 'use_original' : 'fork'
      })
      setRepoDecisions(decisions)

      // Move to access check step
      setCurrentStep('access-check')
      setIsLoading(false)
    } catch (err: any) {
      console.error('Failed to check repo access:', err)
      setError('Failed to check repository access')
      setIsLoading(false)
    }
  }

  // Fork repos that need forking
  const forkRepositories = async () => {
    const reposToFork = Object.entries(repoDecisions)
      .filter(([_, decision]) => decision === 'fork')
      .map(([url]) => url)

    if (reposToFork.length === 0) return true

    try {
      await apiClient.v1SampleProjectsSimpleForkReposCreate({
        sample_project_id: sampleProject?.id || '',
        github_connection_id: selectedConnectionId,
        repositories_to_fork: reposToFork,
      })
      return true
    } catch (err: any) {
      console.error('Failed to fork repos:', err)
      setError('Failed to fork repositories: ' + (err.message || 'Unknown error'))
      return false
    }
  }

  // Create the project
  const createProject = async () => {
    if (!sampleProject) return

    setCurrentStep('creating')
    setIsLoading(true)
    setError(null)

    try {
      // Fork repos if needed
      const needsFork = Object.values(repoDecisions).some(d => d === 'fork')
      if (needsFork) {
        const forkSuccess = await forkRepositories()
        if (!forkSuccess) {
          setIsLoading(false)
          return
        }
      }

      // Create the project
      const response = await apiClient.v1SampleProjectsSimpleForkCreate({
        sample_project_id: sampleProject.id || '',
        project_name: sampleProject.name || '',
        organization_id: organizationId,
        helix_app_id: selectedAgentId,
        github_connection_id: selectedConnectionId,
        repository_decisions: repoDecisions,
      })

      snackbar.success(`Project "${sampleProject.name}" created successfully!`)
      onComplete(response.data.project_id || '')
    } catch (err: any) {
      console.error('Failed to create project:', err)
      setError('Failed to create project: ' + (err.message || 'Unknown error'))
      setIsLoading(false)
    }
  }

  // Render GitHub connection step
  const renderGitHubCheckStep = () => (
    <Box sx={{ py: 2 }}>
      <Alert severity="info" sx={{ mb: 3 }}>
        <Typography variant="body2">
          <strong>{sampleProject?.name}</strong> requires a GitHub connection so you can push changes and create pull requests.
        </Typography>
      </Alert>

      {gitHubConnections.length > 0 ? (
        <Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Select a GitHub account to use:
          </Typography>
          <RadioGroup
            value={selectedConnectionId}
            onChange={(e) => setSelectedConnectionId(e.target.value)}
          >
            {gitHubConnections.map((conn) => (
              <FormControlLabel
                key={conn.id}
                value={conn.id}
                control={<Radio />}
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Github size={18} />
                    <Typography>{conn.provider_username || conn.provider_user_email}</Typography>
                  </Box>
                }
              />
            ))}
          </RadioGroup>
          <Button
            variant="contained"
            onClick={() => checkRepoAccess(selectedConnectionId)}
            disabled={!selectedConnectionId || isLoading}
            sx={{ mt: 2 }}
          >
            {isLoading ? <CircularProgress size={20} /> : 'Continue'}
          </Button>
        </Box>
      ) : (
        <Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            You need to connect your GitHub account first.
          </Typography>
          <Button
            variant="contained"
            startIcon={<Github size={18} />}
            onClick={() => {
              // Navigate to OAuth connections page
              window.location.href = '/oauth-connections?provider=github'
            }}
          >
            Connect GitHub
          </Button>
        </Box>
      )}
    </Box>
  )

  // Render access check step
  const renderAccessCheckStep = () => (
    <Box sx={{ py: 2 }}>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {accessInfo?.github_username ? (
          <>Connected as <strong>@{accessInfo.github_username}</strong></>
        ) : (
          'Checking repository access...'
        )}
      </Typography>

      {accessInfo?.all_have_write_access ? (
        <Alert severity="success" sx={{ mb: 3 }}>
          <Typography variant="body2">
            You have write access to all repositories. You can push directly to helixml!
          </Typography>
        </Alert>
      ) : (
        <Alert severity="warning" sx={{ mb: 3 }}>
          <Typography variant="body2">
            You don't have write access to some repositories. We'll fork them to your account so you can contribute via pull request.
          </Typography>
        </Alert>
      )}

      <List dense>
        {accessInfo?.repositories?.map((repo) => (
          <ListItem key={repo.github_url}>
            <ListItemIcon sx={{ minWidth: 40 }}>
              {repo.has_write_access ? (
                <Unlock size={20} color="#4caf50" />
              ) : (
                <Lock size={20} color="#ff9800" />
              )}
            </ListItemIcon>
            <ListItemText
              primary={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Typography variant="body2">{repo.owner}/{repo.repo}</Typography>
                  {repo.is_primary && (
                    <Chip label="Primary" size="small" color="primary" />
                  )}
                </Box>
              }
              secondary={
                repo.has_write_access
                  ? 'Write access - can push directly'
                  : repo.existing_fork
                  ? `Fork exists at ${repo.existing_fork}`
                  : 'Will fork to your account'
              }
            />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {repo.has_write_access ? (
                <CheckCircle size={20} color="#4caf50" />
              ) : (
                <GitFork size={20} color="#2196f3" />
              )}
            </Box>
          </ListItem>
        ))}
      </List>

      {!accessInfo?.all_have_write_access && (
        <>
          <Divider sx={{ my: 2 }} />
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            Choose how to handle repositories you don't have access to:
          </Typography>
          <RadioGroup
            value={forkDecision}
            onChange={(e) => {
              const decision = e.target.value as 'use_original' | 'fork'
              setForkDecision(decision)
              // Update all non-writable repos to fork
              const newDecisions = { ...repoDecisions }
              accessInfo?.repositories?.forEach((repo) => {
                if (!repo.has_write_access) {
                  newDecisions[repo.github_url || ''] = decision
                }
              })
              setRepoDecisions(newDecisions)
            }}
          >
            <FormControlLabel
              value="fork"
              control={<Radio />}
              label={
                <Box>
                  <Typography variant="body2">Fork to my account (Recommended)</Typography>
                  <Typography variant="caption" color="text.secondary">
                    You'll be able to push changes and create PRs
                  </Typography>
                </Box>
              }
            />
            <FormControlLabel
              value="use_original"
              control={<Radio />}
              label={
                <Box>
                  <Typography variant="body2">Use original repos (read-only)</Typography>
                  <Typography variant="caption" color="text.secondary">
                    You can explore but won't be able to push
                  </Typography>
                </Box>
              }
            />
          </RadioGroup>
        </>
      )}
    </Box>
  )

  // Render creating step
  const renderCreatingStep = () => (
    <Box sx={{ py: 4, textAlign: 'center' }}>
      <CircularProgress size={48} sx={{ mb: 2 }} />
      <Typography variant="h6" sx={{ mb: 1 }}>
        Setting up your project...
      </Typography>
      <Typography variant="body2" color="text.secondary">
        {forkDecision === 'fork' ? 'Forking repositories and ' : ''}
        Creating project configuration
      </Typography>
    </Box>
  )

  // Get step index for stepper
  const getStepIndex = () => {
    switch (currentStep) {
      case 'github-check':
        return 0
      case 'access-check':
        return 1
      case 'fork-decision':
        return 2
      case 'creating':
        return 3
      default:
        return 0
    }
  }

  const canProceed = () => {
    switch (currentStep) {
      case 'github-check':
        return gitHubConnections.length > 0 && selectedConnectionId
      case 'access-check':
        return !!accessInfo
      default:
        return false
    }
  }

  return (
    <Dialog
      open={open}
      onClose={isLoading ? undefined : onClose}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Github size={24} />
          <Typography variant="h6">
            Set up {sampleProject?.name || 'Sample Project'}
          </Typography>
        </Box>
      </DialogTitle>

      <DialogContent>
        <Stepper activeStep={getStepIndex()} sx={{ mb: 3 }}>
          <Step>
            <StepLabel>Connect GitHub</StepLabel>
          </Step>
          <Step>
            <StepLabel>Check Access</StepLabel>
          </Step>
          <Step>
            <StepLabel>Create Project</StepLabel>
          </Step>
        </Stepper>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {currentStep === 'github-check' && renderGitHubCheckStep()}
        {currentStep === 'access-check' && renderAccessCheckStep()}
        {currentStep === 'creating' && renderCreatingStep()}
      </DialogContent>

      <DialogActions>
        <Button onClick={onClose} disabled={isLoading}>
          Cancel
        </Button>

        {currentStep === 'access-check' && (
          <Button
            variant="contained"
            onClick={createProject}
            disabled={isLoading}
          >
            {Object.values(repoDecisions).some(d => d === 'fork')
              ? 'Fork & Create Project'
              : 'Create Project'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  )
}

export default SampleProjectWizard
