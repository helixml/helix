import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { ArrowLeft, Copy, ExternalLink, Globe2, LockKeyhole, Share2 } from 'lucide-react'

import Page from '../components/system/Page'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useGetArtifact, useSetArtifactVisibility } from '../services/artifactService'

const ArtifactViewer: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const artifactId = router.params.artifact_id as string
  const { data: artifact, isLoading, error } = useGetArtifact(artifactId)
  const visibilityMutation = useSetArtifactVisibility(artifactId)
  const [shareAnchor, setShareAnchor] = useState<HTMLElement | null>(null)

  const isPublic = artifact?.visibility === 'public'
  const frameURL = isPublic
    ? artifact?.subdomain_url
    : artifact ? `/artifacts/${artifact.id}/embed` : undefined

  const goBack = () => {
    if (window.history.length > 1) {
      window.history.back()
      return
    }
    if (artifact?.project_id && artifact.organization_id) {
      router.navigate('org_project-artifacts', { org_id: artifact.organization_id, id: artifact.project_id })
    }
  }

  const setVisibility = async (visibility: 'project' | 'public') => {
    try {
      await visibilityMutation.mutateAsync(visibility)
      snackbar.success(visibility === 'public' ? 'Public share link enabled' : 'Artifact is private')
      setShareAnchor(null)
    } catch (mutationError) {
      snackbar.error(mutationError instanceof Error ? mutationError.message : 'Failed to update artifact visibility')
    }
  }

  const copyShareLink = async () => {
    if (!artifact?.subdomain_url) return
    await navigator.clipboard.writeText(artifact.subdomain_url)
    snackbar.success('Public link copied')
    setShareAnchor(null)
  }

  const openShareMenu = (event: MouseEvent<HTMLElement>) => setShareAnchor(event.currentTarget)

  if (isLoading) {
    return <Box sx={{ height: '100%', display: 'grid', placeItems: 'center' }}><CircularProgress /></Box>
  }

  if (!artifact || error) {
    return (
      <Box sx={{ height: '100%', display: 'grid', placeItems: 'center', p: 3 }}>
        <Typography color="text.secondary">You do not have access to this artifact, or it no longer exists.</Typography>
      </Box>
    )
  }

  return (
    <Page
      breadcrumbTitle={artifact.name || 'Artifact'}
      breadcrumbShowHome={false}
      showDrawerButton={false}
      notifications={false}
      themeToggle={false}
      disableContentScroll
      px={1}
      topbarLeftContent={(
        <Tooltip title="Back to artifacts">
          <IconButton aria-label="Back to artifacts" onClick={goBack} sx={{ width: 30, height: 30, color: 'text.secondary' }}>
            <ArrowLeft size={18} />
          </IconButton>
        </Tooltip>
      )}
      topbarContent={(
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip
            size="small"
            variant="outlined"
            color={isPublic ? 'success' : 'default'}
            icon={isPublic ? <Globe2 size={14} /> : <LockKeyhole size={14} />}
            label={isPublic ? 'Public' : 'Private'}
          />
          <Tooltip title="Share artifact">
            <span>
              <IconButton
                aria-label="Share artifact"
                onClick={openShareMenu}
                disabled={visibilityMutation.isPending}
                sx={{ width: 30, height: 30, color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
              >
                <Share2 size={18} />
              </IconButton>
            </span>
          </Tooltip>
        </Stack>
      )}
    >
      <Box sx={{ flex: 1, minHeight: 0, p: 1, bgcolor: 'background.default' }}>
        {frameURL ? (
          <Box
            component="iframe"
            key={`${artifact.id}:${artifact.visibility}:${artifact.active_version_id}`}
            title={artifact.name || 'Artifact preview'}
            src={frameURL}
            sandbox="allow-scripts allow-forms allow-modals allow-popups allow-downloads allow-same-origin"
            referrerPolicy="no-referrer"
            sx={{ display: 'block', width: '100%', height: '100%', border: '1px solid', borderColor: 'divider', borderRadius: 1, bgcolor: '#fff' }}
          />
        ) : (
          <Box sx={{ height: '100%', display: 'grid', placeItems: 'center' }}>
            <Typography color="text.secondary">The public share origin is unavailable.</Typography>
          </Box>
        )}
      </Box>

      <Menu anchorEl={shareAnchor} open={!!shareAnchor} onClose={() => setShareAnchor(null)}>
        {isPublic ? [
          <MenuItem key="copy" onClick={copyShareLink} disabled={!artifact.subdomain_url}>
            <Copy size={20} /><Typography sx={{ ml: 1 }}>Copy public link</Typography>
          </MenuItem>,
          <MenuItem key="open" onClick={() => artifact.subdomain_url && window.open(artifact.subdomain_url, '_blank', 'noopener,noreferrer')} disabled={!artifact.subdomain_url}>
            <ExternalLink size={20} /><Typography sx={{ ml: 1 }}>Open public link</Typography>
          </MenuItem>,
          <MenuItem key="private" onClick={() => setVisibility('project')}>
            <LockKeyhole size={20} /><Typography sx={{ ml: 1 }}>Make private</Typography>
          </MenuItem>,
        ] : (
          <MenuItem onClick={() => setVisibility('public')}>
            <Globe2 size={20} /><Typography sx={{ ml: 1 }}>Publish publicly</Typography>
          </MenuItem>
        )}
      </Menu>
    </Page>
  )
}

export default ArtifactViewer
