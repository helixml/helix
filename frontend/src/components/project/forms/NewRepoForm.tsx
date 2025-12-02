import React, { FC } from 'react'
import {
  TextField,
  Stack,
} from '@mui/material'

interface NewRepoFormProps {
  name: string
  onNameChange: (name: string) => void
  description: string
  onDescriptionChange: (description: string) => void
  size?: 'small' | 'medium'
  autoFocus?: boolean
  /** Custom helper text for the name field */
  nameHelperText?: string
}

/**
 * Reusable form fields for creating a new Helix-hosted repository.
 * Used by both CreateRepositoryDialog and CreateProjectDialog.
 */
const NewRepoForm: FC<NewRepoFormProps> = ({
  name,
  onNameChange,
  description,
  onDescriptionChange,
  size = 'small',
  autoFocus = false,
  nameHelperText = 'A new Helix-hosted repository will be created',
}) => {
  return (
    <Stack spacing={2}>
      <TextField
        label="Repository Name"
        fullWidth
        size={size}
        value={name}
        onChange={(e) => onNameChange(e.target.value)}
        required
        autoFocus={autoFocus}
        helperText={nameHelperText}
      />
      <TextField
        label="Repository Description"
        fullWidth
        size={size}
        value={description}
        onChange={(e) => onDescriptionChange(e.target.value)}
      />
    </Stack>
  )
}

export default NewRepoForm
