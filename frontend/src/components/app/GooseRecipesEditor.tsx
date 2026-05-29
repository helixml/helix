import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'

import { IAssistantGooseRecipe } from '../../types'

const SLASH_COMMAND_PATTERN = /^[a-z0-9][a-z0-9_-]*$/

interface GooseRecipesEditorProps {
  recipeRepoURL: string
  recipes: IAssistantGooseRecipe[]
  onChange: (next: { recipeRepoURL: string; recipes: IAssistantGooseRecipe[] }) => void
  disabled?: boolean
}

const GooseRecipesEditor: FC<GooseRecipesEditorProps> = ({
  recipeRepoURL,
  recipes,
  onChange,
  disabled = false,
}) => {
  const [draftName, setDraftName] = useState('')
  const [draftPath, setDraftPath] = useState('')

  const draftNameValid = draftName === '' || SLASH_COMMAND_PATTERN.test(draftName)
  const draftPathValid = draftPath === '' || !draftPath.startsWith('/')
  const canAdd =
    !disabled &&
    draftName.trim() !== '' &&
    draftPath.trim() !== '' &&
    draftNameValid &&
    draftPathValid &&
    !recipes.some((r) => r.name === draftName.trim())

  const handleAdd = () => {
    if (!canAdd) return
    onChange({
      recipeRepoURL,
      recipes: [...recipes, { name: draftName.trim(), path: draftPath.trim() }],
    })
    setDraftName('')
    setDraftPath('')
  }

  const handleRemove = (idx: number) => {
    onChange({
      recipeRepoURL,
      recipes: recipes.filter((_, i) => i !== idx),
    })
  }

  return (
    <Box>
      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
        Goose Recipes
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
        Recipes become slash commands inside the Goose thread. Paths are
        repo-relative to the recipe repository below (or the project's
        primary repository when no recipe repo URL is set). The repository
        must already be attached to the project on the Repositories tab.
      </Typography>

      <TextField
        label="Recipe repository URL (optional)"
        value={recipeRepoURL}
        onChange={(e) => onChange({ recipeRepoURL: e.target.value, recipes })}
        fullWidth
        size="small"
        placeholder="https://github.com/your-org/your-recipes"
        helperText="Leave empty to load recipes from the primary repo. URL must match an attached repository."
        disabled={disabled}
        sx={{ mb: 2 }}
      />

      {recipes.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          No recipes yet. Add one below.
        </Typography>
      ) : (
        <Box sx={{ mb: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
          {recipes.map((recipe, idx) => (
            <Box
              key={`${recipe.name}-${idx}`}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                p: 1,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
              }}
            >
              <Typography variant="body2" sx={{ fontFamily: 'monospace', minWidth: 140 }}>
                /{recipe.name}
              </Typography>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontFamily: 'monospace', flex: 1, wordBreak: 'break-all' }}
              >
                {recipe.path}
              </Typography>
              <IconButton
                size="small"
                onClick={() => handleRemove(idx)}
                disabled={disabled}
                aria-label={`Remove recipe ${recipe.name}`}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Box>
          ))}
        </Box>
      )}

      <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
        <TextField
          label="Slash command"
          value={draftName}
          onChange={(e) => setDraftName(e.target.value)}
          size="small"
          placeholder="triage"
          disabled={disabled}
          error={!draftNameValid}
          helperText={
            !draftNameValid
              ? 'Use lowercase letters, digits, underscore, dash'
              : 'Name without the leading "/"'
          }
          sx={{ minWidth: 200 }}
        />
        <TextField
          label="Recipe path"
          value={draftPath}
          onChange={(e) => setDraftPath(e.target.value)}
          size="small"
          placeholder=".goose/recipes/triage.yaml"
          disabled={disabled}
          error={!draftPathValid}
          helperText={
            !draftPathValid
              ? 'Use a repo-relative path (no leading slash)'
              : 'Path inside the recipe repository'
          }
          sx={{ flex: 1 }}
        />
        <Button
          startIcon={<AddIcon />}
          onClick={handleAdd}
          disabled={!canAdd}
          variant="outlined"
          size="small"
          sx={{ mt: 0.25 }}
        >
          Add
        </Button>
      </Box>
    </Box>
  )
}

export default GooseRecipesEditor
