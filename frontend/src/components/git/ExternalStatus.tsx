import React, { FC, useState } from 'react'
import {
  Typography,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  CircularProgress,
  Alert,
} from '@mui/material'
import { ArrowDown, RefreshCw } from 'lucide-react'
import { useListRepositoryCommits, usePullFromRemote } from '../../services/gitRepositoryService'
import useSnackbar from '../../hooks/useSnackbar'

interface ExternalStatusProps {
  repositoryId: string
  branch: string
  isExternal: boolean
}

const ExternalStatus: FC<ExternalStatusProps> = ({
  repositoryId,
  branch,
  isExternal,
}) => {
  const snackbar = useSnackbar()
  const [dialogOpen, setDialogOpen] = useState(false)
  
  const { data: commitsData, isLoading } = useListRepositoryCommits(
    repositoryId,
    branch || undefined,
    1,
    100
  )
  
  const pullFromRemoteMutation = usePullFromRemote()
  
  const commitsBehind = commitsData?.external_status?.commits_behind || 0
  
  if (!isExternal || isLoading || commitsBehind === 0) {
    return null
  }
  
  const handlePull = async () => {
    try {
      await pullFromRemoteMutation.mutateAsync({
        repositoryId,
        branch,
        force: false,
      })
      snackbar.success('Successfully pulled from remote')
      setDialogOpen(false)
    } catch (error: any) {
      console.error('Failed to pull from remote:', error)
      let errorMessage = 'Failed to pull from remote'
      if (error?.response?.data) {
        if (typeof error.response.data === 'string') {
          errorMessage = error.response.data
        } else {
          errorMessage = error.response.data.error || error.response.data.message || errorMessage
        }
      } else if (error?.message) {
        errorMessage = error.message
      }
      snackbar.error(errorMessage)
    }
  }
  
  return (
    <>
      <Typography
        variant="body2"
        color="secondary"
        onClick={() => setDialogOpen(true)}
        sx={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 0.5 }}
      >
        <ArrowDown size={14} />
        {commitsBehind} commit{commitsBehind !== 1 ? 's' : ''} behind
      </Typography>
      
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>          
          Pull from Remote
        </DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            Your local branch is {commitsBehind} commit{commitsBehind !== 1 ? 's' : ''} behind the remote.
          </Alert>
          <Typography variant="body2">
            Click "Pull" to fetch the latest changes from the remote repository for branch <strong>{branch}</strong>.
          </Typography>
        </DialogContent>
        <DialogActions>          
          <Button
            onClick={handlePull}
            variant="contained"
            color="secondary"
            disabled={pullFromRemoteMutation.isPending}
            startIcon={pullFromRemoteMutation.isPending ? <CircularProgress size={16} /> : <ArrowDown size={16} />}
          >
            {pullFromRemoteMutation.isPending ? 'Pulling...' : 'Pull'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default ExternalStatus
