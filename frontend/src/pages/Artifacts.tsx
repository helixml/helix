import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Plus } from 'lucide-react'

import { TypesArtifact } from '../api/api'
import ArtifactDialog from '../components/artifacts/ArtifactDialog'
import ArtifactsView from '../components/artifacts/ArtifactsView'
import Page from '../components/system/Page'
import PageSectionHeader from '../components/system/PageSectionHeader'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import ViewModeToggle from '../components/widgets/ViewModeToggle'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useViewMode from '../hooks/useViewMode'
import { useGetProject } from '../services/projectService'
import {
  ArtifactForm,
  useCreateArtifact,
  useDeleteArtifact,
  useListProjectArtifacts,
  useUpdateArtifact,
} from '../services/artifactService'

const errorMessage = (error: unknown) => {
  if (error instanceof Error) return error.message
  return 'Artifact request failed'
}

const Artifacts: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const projectId = router.params.id as string
  const [viewMode, setViewMode] = useViewMode('artifacts-view-mode', 'table')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<TypesArtifact>()
  const [deleting, setDeleting] = useState<TypesArtifact>()
  const [saveError, setSaveError] = useState<string>()

  const { data: project } = useGetProject(projectId, !!projectId)
  const { data: artifacts = [], isLoading } = useListProjectArtifacts(projectId)
  const createMutation = useCreateArtifact(projectId)
  const updateMutation = useUpdateArtifact(projectId)
  const deleteMutation = useDeleteArtifact(projectId)
  const saving = createMutation.isPending || updateMutation.isPending

  const openCreate = () => {
    setEditing(undefined)
    setSaveError(undefined)
    setDialogOpen(true)
  }

  const openEdit = (artifact: TypesArtifact) => {
    setEditing(artifact)
    setSaveError(undefined)
    setDialogOpen(true)
  }

  const saveArtifact = async (form: ArtifactForm) => {
    try {
      if (editing?.id) {
        await updateMutation.mutateAsync({ id: editing.id, form })
        snackbar.success(`Artifact ${form.name} updated`)
      } else {
        await createMutation.mutateAsync(form)
        snackbar.success(`Artifact ${form.name} created`)
      }
      setDialogOpen(false)
      setEditing(undefined)
    } catch (error) {
      setSaveError(errorMessage(error))
    }
  }

  const deleteArtifact = async () => {
    if (!deleting?.id) return
    try {
      await deleteMutation.mutateAsync(deleting.id)
      snackbar.success(`Artifact ${deleting.name || deleting.id} deleted`)
      setDeleting(undefined)
    } catch (error) {
      snackbar.error(errorMessage(error))
    }
  }

  return (
    <Page
      breadcrumbTitle="Artifacts"
      orgBreadcrumbs
      breadcrumbs={project?.name ? [{
        title: project.name,
        routeName: 'org_project-specs',
        params: { id: projectId },
      }] : []}
    >
      <Container maxWidth="lg" sx={{ mb: 4, mt: 4 }}>
        <PageSectionHeader
          title="Artifacts"
          description="Static HTML pages and compiled apps stored in this project. They inherit project access and run without a sandbox."
          action={(
            <Button variant="contained" color="secondary" startIcon={<Plus size={18} />} onClick={openCreate}>
              New Artifact
            </Button>
          )}
        />
        <Stack spacing={2}>
          {isLoading ? (
            <LoadingSpinner />
          ) : artifacts.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary">No artifacts have been published in this project.</Typography>
              <Button variant="contained" color="secondary" startIcon={<Plus size={18} />} onClick={openCreate} sx={{ mt: 2 }}>
                Publish the first artifact
              </Button>
            </Box>
          ) : (
            <>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <ViewModeToggle mode={viewMode} onChange={setViewMode} />
              </Box>
              <ArtifactsView artifacts={artifacts} mode={viewMode} onEdit={openEdit} onDelete={setDeleting} />
            </>
          )}
        </Stack>
      </Container>

      <ArtifactDialog
        open={dialogOpen}
        artifact={editing}
        saving={saving}
        error={saveError}
        onClose={() => setDialogOpen(false)}
        onSubmit={saveArtifact}
      />
      {deleting && (
        <DeleteConfirmWindow
          title={`artifact ${deleting.name || deleting.id}`}
          onCancel={() => setDeleting(undefined)}
          onSubmit={deleteArtifact}
        />
      )}
    </Page>
  )
}

export default Artifacts
