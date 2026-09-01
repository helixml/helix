import { ChangeEvent, FC, useEffect, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { Upload } from 'lucide-react'

import { TypesArtifact, TypesArtifactKind } from '../../api/api'
import { ArtifactForm } from '../../services/artifactService'
import MarkdownCodeBlock from '../session/MarkdownCodeBlock'

type Props = {
  open: boolean
  artifact?: TypesArtifact
  projectName?: string
  saving: boolean
  error?: string
  onClose: () => void
  onSubmit: (form: ArtifactForm) => Promise<void>
}

const ArtifactDialog: FC<Props> = ({ open, artifact, projectName, saving, error, onClose, onSubmit }) => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [entrypoint, setEntrypoint] = useState('')
  const [visibility, setVisibility] = useState<'project' | 'public'>('project')
  const [file, setFile] = useState<File>()

  useEffect(() => {
    if (!open) return
    setName(artifact?.name ?? '')
    setDescription(artifact?.description ?? '')
    setEntrypoint(artifact?.entrypoint ?? '')
    setVisibility(artifact?.visibility === 'public' ? 'public' : 'project')
    setFile(undefined)
  }, [open, artifact?.id])

  const handleFile = (event: ChangeEvent<HTMLInputElement>) => {
    setFile(event.target.files?.[0])
  }

  const canSave = !!name.trim() && (!!artifact || !!file) && !saving
  const fileExtension = file?.name.split('.').pop()?.toLowerCase()
  const selectedDocument = !!file && (
    file.type === 'application/pdf' ||
    file.type.startsWith('image/') ||
    file.type === 'text/markdown' ||
    file.type === 'text/x-markdown' ||
    ['pdf', 'png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico', 'avif'].includes(fileExtension ?? '') ||
    ['md', 'markdown'].includes(fileExtension ?? '')
  )
  const documentArtifact = artifact?.kind === TypesArtifactKind.ArtifactKindPDF ||
    artifact?.kind === TypesArtifactKind.ArtifactKindImage ||
    artifact?.kind === TypesArtifactKind.ArtifactKindMarkdown
  const showEntrypoint = file ? !selectedDocument : !documentArtifact
  const agentPrompt = projectName
    ? `Create and upload an artifact to the Helix project "${projectName}".`
    : 'Create and upload an artifact to this Helix project.'

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} fullWidth maxWidth={artifact ? 'sm' : 'md'}>
      <DialogTitle>{artifact ? 'Update artifact' : 'New artifact'}</DialogTitle>
      <DialogContent>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: artifact ? '1fr' : { xs: '1fr', md: 'minmax(0, 1fr) auto minmax(280px, 0.8fr)' },
            gap: 3,
            pt: 1,
          }}
        >
          <Stack spacing={2}>
            {!artifact && (
              <Box>
                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Upload manually</Typography>
                <Typography variant="body2" color="text.secondary">
                  Upload HTML, Markdown, a compiled app ZIP, a PDF, or an image.
                </Typography>
              </Box>
            )}
            {error && <Alert severity="error">{error}</Alert>}
            <TextField label="Name" value={name} onChange={(event) => setName(event.target.value)} required autoFocus />
            <TextField label="Description" value={description} onChange={(event) => setDescription(event.target.value)} multiline minRows={2} />
            <TextField
              select
              label="Visibility"
              value={visibility}
              onChange={(event) => setVisibility(event.target.value as 'project' | 'public')}
              helperText={visibility === 'public' ? 'Public artifacts receive an isolated share subdomain.' : 'Only project members can open this artifact.'}
            >
              <MenuItem value="project">Project members</MenuItem>
              <MenuItem value="public">Public link</MenuItem>
            </TextField>
            {showEntrypoint && (
              <TextField
                label="Entrypoint"
                value={entrypoint}
                onChange={(event) => setEntrypoint(event.target.value)}
                placeholder="index.html"
                helperText="Leave blank to use index.html. Only HTML and ZIP artifacts need an entrypoint."
              />
            )}
            <Button component="label" variant="outlined" startIcon={<Upload size={18} />} sx={{ alignSelf: 'flex-start' }}>
              {artifact ? 'Publish new version' : 'Choose artifact file'}
              <input hidden type="file" accept=".html,.htm,.md,.markdown,.zip,.pdf,image/*,text/html,text/markdown,application/zip,application/pdf" onChange={handleFile} />
            </Button>
            <Typography variant="body2" color="text.secondary">
              {file?.name ?? (artifact ? 'Keep the current content version' : 'No file selected')}
            </Typography>
          </Stack>

          {!artifact && (
            <Box
              sx={{
                display: 'flex',
                flexDirection: { xs: 'row', md: 'column' },
                alignItems: 'center',
                minWidth: { md: 24 },
                minHeight: { md: '100%' },
              }}
            >
              <Box sx={{ flex: 1, width: { xs: 'auto', md: '1px' }, height: { xs: '1px', md: 'auto' }, bgcolor: 'divider' }} />
              <Typography variant="caption" color="text.secondary" sx={{ px: { xs: 1.5, md: 0 }, py: { xs: 0, md: 1 } }}>
                Or
              </Typography>
              <Box sx={{ flex: 1, width: { xs: 'auto', md: '1px' }, height: { xs: '1px', md: 'auto' }, bgcolor: 'divider' }} />
            </Box>
          )}

          {!artifact && (
            <Box sx={{ alignSelf: 'start', minWidth: 0 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Create with an agent</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                Tell an agent with access to this project what you want to build. It can create and upload the finished artifact here for you.
              </Typography>
              <MarkdownCodeBlock language="text" defaultWrapped>{agentPrompt}</MarkdownCodeBlock>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button
          variant="contained"
          color="secondary"
          disabled={!canSave}
          onClick={() => onSubmit({
            name: name.trim(),
            description,
            entrypoint: showEntrypoint ? (entrypoint || undefined) : undefined,
            visibility,
            artifact: file,
          })}
        >
          {saving ? 'Saving…' : artifact ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ArtifactDialog
