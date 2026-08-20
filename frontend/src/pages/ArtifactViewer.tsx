import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Copy, ExternalLink, Globe2, LockKeyhole } from 'lucide-react'

import { TypesArtifactKind } from '../api/api'
import ArtifactVisibilityBadge from '../components/artifacts/ArtifactVisibilityBadge'
import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useGetArtifactViewer, useSetArtifactVisibility } from '../services/artifactService'

const ArtifactViewer: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  const artifactId = router.params.artifact_id as string
  const { data: artifact, isLoading, error } = useGetArtifactViewer(artifactId)
  const visibilityMutation = useSetArtifactVisibility(artifactId)
  const [shareAnchor, setShareAnchor] = useState<HTMLElement | null>(null)

  const organizationSlug = artifact?.organization_name || artifact?.organization_id || ''
  const organizationTitle = artifact?.organization_name || 'Organization'

  const isPublic = artifact?.visibility === 'public'
  const isImage = artifact?.kind === TypesArtifactKind.ArtifactKindImage
  const isPDF = artifact?.kind === TypesArtifactKind.ArtifactKindPDF
  const frameURL = isPDF
    ? `/artifacts/${artifact.id}/document`
    : isPublic
      ? artifact?.subdomain_url
      : artifact ? `/artifacts/${artifact.id}/embed` : undefined

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

  const login = () => {
    localStorage.setItem('login_redirect_url', window.location.pathname + window.location.search)
    router.navigate('login')
  }

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
      breadcrumbs={[
        {
          title: organizationTitle,
          routeName: 'org_projects',
          params: { org_id: organizationSlug },
        },
        {
          title: 'Projects',
          routeName: 'org_projects',
          params: { org_id: organizationSlug },
        },
        {
          title: artifact.project_name || 'Project',
          routeName: 'org_project-specs',
          params: { org_id: organizationSlug, id: artifact.project_id },
        },
        {
          title: 'artifacts',
          routeName: 'org_project-artifacts',
          params: { org_id: organizationSlug, id: artifact.project_id },
        },
      ]}
      breadcrumbTitle={artifact.name || 'Artifact'}
      showDrawerButton={false}
      notifications={false}
      themeToggle={false}
      disableContentScroll
      px={3}
      topbarContent={(
        <Stack direction="row" spacing={1} alignItems="center">
          <ArtifactVisibilityBadge visibility={artifact.visibility} privateLabel="Private" />
          {account.initialized && !account.user ? (
            <Button size="small" onClick={login} sx={{ textTransform: 'none', fontWeight: 500, minWidth: 'auto' }}>
              Login
            </Button>
          ) : artifact.can_edit ? (
            <Button
              size="small"
              onClick={openShareMenu}
              disabled={visibilityMutation.isPending}
              sx={{ textTransform: 'none', fontWeight: 500, minWidth: 'auto' }}
            >
              Share
            </Button>
          ) : null}
        </Stack>
      )}
    >
      <Box sx={{ flex: 1, minHeight: 0, p: 1, bgcolor: 'background.default' }}>
        {frameURL && isImage ? (
          <Box sx={{ width: '100%', height: '100%', display: 'grid', placeItems: 'center', overflow: 'auto' }}>
            <Box
              component="img"
              src={frameURL}
              alt={artifact.name || 'Artifact image'}
              sx={{ display: 'block', maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }}
            />
          </Box>
        ) : frameURL ? (
          <Box
            component="iframe"
            key={`${artifact.id}:${artifact.visibility}:${artifact.active_version_id}`}
            title={artifact.name || 'Artifact preview'}
            src={frameURL}
            sandbox={isPDF ? undefined : 'allow-scripts allow-forms allow-modals allow-popups allow-downloads allow-same-origin'}
            referrerPolicy="no-referrer"
            sx={{ display: 'block', width: '100%', height: '100%', border: 0, bgcolor: '#fff' }}
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
            <ListItemIcon><Copy size={16} /></ListItemIcon>
            <ListItemText>Copy public link</ListItemText>
          </MenuItem>,
          <MenuItem key="open" onClick={() => artifact.subdomain_url && window.open(artifact.subdomain_url, '_blank', 'noopener,noreferrer')} disabled={!artifact.subdomain_url}>
            <ListItemIcon><ExternalLink size={16} /></ListItemIcon>
            <ListItemText>Open public link</ListItemText>
          </MenuItem>,
          <MenuItem key="private" onClick={() => setVisibility('project')}>
            <ListItemIcon><LockKeyhole size={16} /></ListItemIcon>
            <ListItemText>Make private</ListItemText>
          </MenuItem>,
        ] : (
          <MenuItem onClick={() => setVisibility('public')}>
            <ListItemIcon><Globe2 size={16} /></ListItemIcon>
            <ListItemText>Publish publicly</ListItemText>
          </MenuItem>
        )}
      </Menu>
    </Page>
  )
}

export default ArtifactViewer
