import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import InputAdornment from '@mui/material/InputAdornment'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { Plus, Search } from 'lucide-react'

import { TypesArtifact, TypesArtifactKind, TypesArtifactVisibility } from '../api/api'
import ArtifactDialog from '../components/artifacts/ArtifactDialog'
import ArtifactsView, { artifactKindLabel } from '../components/artifacts/ArtifactsView'
import Page from '../components/system/Page'
import PageSectionHeader from '../components/system/PageSectionHeader'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import ViewModeToggle from '../components/widgets/ViewModeToggle'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useViewMode from '../hooks/useViewMode'
import { useGetProject } from '../services/projectService'
import { matchesAllTokens } from '../utils/searchUtils'
import {
  ArtifactForm,
  useCreateArtifact,
  useDeleteArtifact,
  useListProjectArtifacts,
  useUpdateArtifact,
} from '../services/artifactService'

const KIND_OPTIONS: { value: string, label: string }[] = [
  { value: 'all', label: 'All types' },
  ...Object.values(TypesArtifactKind).map((kind) => ({
    value: kind,
    label: artifactKindLabel({ kind }),
  })),
]

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
  const [query, setQuery] = useState('')
  const [kindFilter, setKindFilter] = useState('all')
  const [visibilityFilter, setVisibilityFilter] = useState('all')

  const { data: project } = useGetProject(projectId, !!projectId)
  const { data: artifacts = [], isLoading } = useListProjectArtifacts(projectId)
  const createMutation = useCreateArtifact(projectId)
  const updateMutation = useUpdateArtifact(projectId)
  const deleteMutation = useDeleteArtifact(projectId)
  const saving = createMutation.isPending || updateMutation.isPending

  const filtered = useMemo(() => artifacts.filter((artifact) => {
    // Artifacts uploaded before a kind was inferred fall back to single_file, the
    // same default the server applies.
    if (kindFilter !== 'all' && (artifact.kind || TypesArtifactKind.ArtifactKindSingleFile) !== kindFilter) return false
    if (visibilityFilter !== 'all' && (artifact.visibility || TypesArtifactVisibility.ArtifactVisibilityProject) !== visibilityFilter) return false
    return matchesAllTokens(query, artifact.name, artifact.description, artifact.id, artifactKindLabel(artifact))
  }), [artifacts, query, kindFilter, visibilityFilter])

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
      breadcrumbs={project?.name ? [
        { title: 'Projects', routeName: 'projects' },
        {
          title: project.name,
          routeName: 'project-specs',
          params: { id: projectId },
        },
      ] : []}
    >
      <Container maxWidth="lg" sx={{ mb: 4, mt: 4 }}>
        <PageSectionHeader
          title="Artifacts"
          description="Artifacts are static pages and apps your agents create and upload to this project. They inherit project access and are served directly by Helix without a running sandbox."
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
              <Typography variant="body1" color="text.secondary">
                No artifacts yet. Ask an agent to create and upload one, or publish HTML or a compiled app manually.
              </Typography>
              <Button variant="contained" color="secondary" startIcon={<Plus size={18} />} onClick={openCreate} sx={{ mt: 2 }}>
                Publish the first artifact
              </Button>
            </Box>
          ) : (
            <>
              <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between" flexWrap="wrap" useFlexGap>
                <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                  <TextField
                    size="small"
                    placeholder="Search artifacts"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    sx={{ width: 260 }}
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <Search size={16} />
                        </InputAdornment>
                      ),
                    }}
                  />
                  <TextField
                    select
                    size="small"
                    SelectProps={{ 'aria-label': 'Filter by type' }}
                    value={kindFilter}
                    onChange={(event) => setKindFilter(event.target.value)}
                    sx={{ width: 180 }}
                  >
                    {KIND_OPTIONS.map((option) => (
                      <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>
                    ))}
                  </TextField>
                  <TextField
                    select
                    size="small"
                    SelectProps={{ 'aria-label': 'Filter by visibility' }}
                    value={visibilityFilter}
                    onChange={(event) => setVisibilityFilter(event.target.value)}
                    sx={{ width: 160 }}
                  >
                    <MenuItem value="all">All visibility</MenuItem>
                    <MenuItem value={TypesArtifactVisibility.ArtifactVisibilityProject}>Project</MenuItem>
                    <MenuItem value={TypesArtifactVisibility.ArtifactVisibilityPublic}>Public</MenuItem>
                  </TextField>
                </Stack>
                <ViewModeToggle mode={viewMode} onChange={setViewMode} />
              </Stack>
              <ArtifactsView
                artifacts={filtered}
                mode={viewMode}
                emptyMessage="No artifacts match your search or filters."
                onEdit={openEdit}
                onDelete={setDeleting}
              />
            </>
          )}
        </Stack>
      </Container>

      <ArtifactDialog
        open={dialogOpen}
        artifact={editing}
        projectName={project?.name}
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
