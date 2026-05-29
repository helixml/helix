import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Autocomplete from '@mui/material/Autocomplete'
import Alert from '@mui/material/Alert'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import { useQuery } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import { IAssistantGooseRecipe } from '../../types'
import { ServerGooseRecipeCandidate } from '../../api/api'

const SLASH_COMMAND_PATTERN = /^[a-z0-9][a-z0-9_-]*$/

// Mirrors goose.DefaultName in the backend — keep in sync. Strips
// .yaml/.yml from the basename of the path.
const deriveRecipeName = (path: string): string => {
  if (!path) return ''
  const base = path.replace(/^.*\//, '').trim()
  if (!base || base === '.' || base === '/') return ''
  const lower = base.toLowerCase()
  if (lower.endsWith('.yaml')) return base.slice(0, -5)
  if (lower.endsWith('.yml')) return base.slice(0, -4)
  return base
}

interface GooseRecipesEditorProps {
  // appId is needed to fetch the list of candidate recipe files from the
  // attached repo. When empty, the picker falls back to plain text input
  // and the user has to type paths by hand — used when the editor is
  // rendered before the app has been saved.
  appId?: string
  recipeRepoURL: string
  recipes: IAssistantGooseRecipe[]
  onChange: (next: { recipeRepoURL: string; recipes: IAssistantGooseRecipe[] }) => void
  disabled?: boolean
}

const GooseRecipesEditor: FC<GooseRecipesEditorProps> = ({
  appId,
  recipeRepoURL,
  recipes,
  onChange,
  disabled = false,
}) => {
  const api = useApi()
  const [draftPath, setDraftPath] = useState('')
  const [draftNameOverride, setDraftNameOverride] = useState('')

  const { data: candidatesResponse, isLoading: loadingCandidates } = useQuery({
    queryKey: ['app-goose-recipe-candidates', appId],
    queryFn: async () => {
      if (!appId) return null
      const response = await api
        .getApiClient()
        .v1AppsGooseRecipesCandidatesDetail(appId)
      return response.data
    },
    enabled: !!appId,
    staleTime: 15000,
  })

  const candidates: ServerGooseRecipeCandidate[] = useMemo(
    () => candidatesResponse?.files || [],
    [candidatesResponse],
  )
  const candidatesError = candidatesResponse?.error || ''

  // Filter out files already added so the picker doesn't suggest duplicates.
  const availableCandidates = useMemo(() => {
    const usedPaths = new Set(recipes.map((r) => r.path))
    return candidates.filter((c) => c.path && !usedPaths.has(c.path))
  }, [candidates, recipes])

  const derivedName = useMemo(() => deriveRecipeName(draftPath), [draftPath])
  const effectiveName = (draftNameOverride.trim() || derivedName).trim()

  const draftNameValid = effectiveName === '' || SLASH_COMMAND_PATTERN.test(effectiveName)
  const draftPathValid = draftPath === '' || !draftPath.startsWith('/')
  const duplicateName = recipes.some((r) => r.name === effectiveName)

  const canAdd =
    !disabled &&
    draftPath.trim() !== '' &&
    draftPathValid &&
    effectiveName !== '' &&
    draftNameValid &&
    !duplicateName

  const handleAdd = () => {
    if (!canAdd) return
    onChange({
      recipeRepoURL,
      recipes: [...recipes, { name: effectiveName, path: draftPath.trim() }],
    })
    setDraftPath('')
    setDraftNameOverride('')
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
        Recipes become slash commands inside the Goose thread and appear as a
        dropdown on the spec-task form. Pick a YAML file from the attached
        recipe repository below (or the project's primary repository when no
        recipe repo URL is set).
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

      {candidatesError && (
        <Alert severity="warning" variant="outlined" sx={{ mb: 1.5 }}>
          {candidatesError}
        </Alert>
      )}

      {recipes.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          No recipes yet. Pick one below.
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

      <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <Autocomplete
          freeSolo
          disablePortal
          loading={loadingCandidates}
          options={availableCandidates}
          getOptionLabel={(option) =>
            typeof option === 'string' ? option : option.path || ''
          }
          renderOption={(props, option) => (
            <li {...props} key={option.path}>
              <Box>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {option.path}
                </Typography>
                {option.title && (
                  <Typography variant="caption" color="text.secondary">
                    {option.title}
                  </Typography>
                )}
              </Box>
            </li>
          )}
          value={draftPath}
          onChange={(_, value) => {
            if (typeof value === 'string') {
              setDraftPath(value)
            } else if (value) {
              setDraftPath(value.path || '')
            } else {
              setDraftPath('')
            }
          }}
          onInputChange={(_, value, reason) => {
            if (reason === 'input') setDraftPath(value)
          }}
          sx={{ minWidth: 340, flex: 1 }}
          renderInput={(params) => (
            <TextField
              {...params}
              label="Recipe file"
              size="small"
              placeholder=".goose/recipes/triage.yaml"
              error={!draftPathValid}
              helperText={
                !draftPathValid
                  ? 'Use a repo-relative path (no leading slash)'
                  : appId
                    ? 'Pick a YAML file from the attached repo, or type a path'
                    : 'Type a repo-relative path'
              }
              disabled={disabled}
            />
          )}
        />
        <TextField
          label="Slash command (optional)"
          value={draftNameOverride}
          onChange={(e) => setDraftNameOverride(e.target.value)}
          size="small"
          placeholder={derivedName || 'release-notes'}
          disabled={disabled}
          error={!draftNameValid || duplicateName}
          helperText={
            !draftNameValid
              ? 'Use lowercase letters, digits, underscore, dash'
              : duplicateName
                ? `Already added as /${effectiveName}`
                : derivedName
                  ? `Defaults to /${derivedName}`
                  : 'Auto-derived from filename'
          }
          sx={{ minWidth: 200 }}
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
