import { ChangeEvent, FC, useEffect, useState } from 'react'
import Alert from '@mui/material/Alert'
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

import { TypesArtifact } from '../../api/api'
import { ArtifactForm } from '../../services/artifactService'

type Props = {
  open: boolean
  artifact?: TypesArtifact
  saving: boolean
  error?: string
  onClose: () => void
  onSubmit: (form: ArtifactForm) => Promise<void>
}

const ArtifactDialog: FC<Props> = ({ open, artifact, saving, error, onClose, onSubmit }) => {
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

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} fullWidth maxWidth="sm">
      <DialogTitle>{artifact ? 'Update artifact' : 'New artifact'}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
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
          <TextField
            label="Entrypoint"
            value={entrypoint}
            onChange={(event) => setEntrypoint(event.target.value)}
            placeholder="index.html"
            helperText="Leave blank when creating to use index.html."
          />
          <Button component="label" variant="outlined" startIcon={<Upload size={18} />} sx={{ alignSelf: 'flex-start' }}>
            {artifact ? 'Publish new version' : 'Choose HTML or ZIP'}
            <input hidden type="file" accept=".html,.htm,.zip,text/html,application/zip" onChange={handleFile} />
          </Button>
          <Typography variant="body2" color="text.secondary">
            {file?.name ?? (artifact ? 'Keep the current content version' : 'No file selected')}
          </Typography>
        </Stack>
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
            entrypoint: entrypoint || undefined,
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
