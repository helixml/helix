import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import Collapse from '@mui/material/Collapse'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Link from '@mui/material/Link'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import FolderIcon from '@mui/icons-material/Folder'
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
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

// TreeNode is the recursive shape we render. We build it once from the
// flat `files: []` list returned by the API — `/` is the implicit
// separator and intermediate directories get folder nodes even when
// no recipe file lives at that depth.
interface TreeNode {
  name: string
  path: string
  isFile: boolean
  title?: string
  children: TreeNode[]
}

const buildTree = (files: ServerGooseRecipeCandidate[]): TreeNode => {
  const root: TreeNode = { name: '', path: '', isFile: false, children: [] }
  for (const file of files) {
    if (!file.path) continue
    const parts = file.path.split('/').filter(Boolean)
    let cursor = root
    parts.forEach((part, idx) => {
      const isLast = idx === parts.length - 1
      const partPath = parts.slice(0, idx + 1).join('/')
      let next = cursor.children.find((c) => c.name === part)
      if (!next) {
        next = {
          name: part,
          path: partPath,
          isFile: isLast,
          title: isLast ? file.title || undefined : undefined,
          children: [],
        }
        cursor.children.push(next)
      }
      cursor = next
    })
  }
  // Sort: folders before files, alphabetical within each group.
  const sortTree = (node: TreeNode) => {
    node.children.sort((a, b) => {
      if (a.isFile !== b.isFile) return a.isFile ? 1 : -1
      return a.name.localeCompare(b.name)
    })
    node.children.forEach(sortTree)
  }
  sortTree(root)
  return root
}

// FileTree renders a TreeNode recursively. Folders track their own
// expanded state; files trigger onSelect when clicked. Disabled paths
// (already added) render greyed-out and unclickable.
interface FileTreeProps {
  node: TreeNode
  depth: number
  selectedPath: string
  disabledPaths: Set<string>
  initiallyExpanded: Set<string>
  onSelect: (path: string) => void
}

const FileTreeNode: FC<FileTreeProps> = ({
  node,
  depth,
  selectedPath,
  disabledPaths,
  initiallyExpanded,
  onSelect,
}) => {
  const [expanded, setExpanded] = useState(() => initiallyExpanded.has(node.path))

  if (node.isFile) {
    const disabled = disabledPaths.has(node.path)
    const selected = selectedPath === node.path
    return (
      <Box
        onClick={() => {
          if (!disabled) onSelect(node.path)
        }}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          pl: depth * 2 + 1,
          pr: 1,
          py: 0.5,
          cursor: disabled ? 'default' : 'pointer',
          color: disabled ? 'text.disabled' : 'text.primary',
          backgroundColor: selected ? 'action.selected' : 'transparent',
          '&:hover': disabled
            ? undefined
            : { backgroundColor: 'action.hover' },
          borderRadius: 0.5,
        }}
      >
        <InsertDriveFileOutlinedIcon fontSize="small" sx={{ opacity: 0.7 }} />
        <Typography variant="body2" sx={{ fontFamily: 'monospace', flex: 1 }}>
          {node.name}
        </Typography>
        {node.title && (
          <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
            {node.title}
          </Typography>
        )}
        {disabled && (
          <CheckCircleIcon fontSize="small" sx={{ opacity: 0.6, color: 'success.main' }} />
        )}
      </Box>
    )
  }

  return (
    <Box>
      <Box
        onClick={() => setExpanded((v) => !v)}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          pl: depth * 2,
          pr: 1,
          py: 0.5,
          cursor: 'pointer',
          '&:hover': { backgroundColor: 'action.hover' },
          borderRadius: 0.5,
        }}
      >
        {expanded ? (
          <ExpandMoreIcon fontSize="small" />
        ) : (
          <ChevronRightIcon fontSize="small" />
        )}
        <FolderIcon fontSize="small" sx={{ opacity: 0.7 }} />
        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
          {node.name || '/'}
        </Typography>
      </Box>
      <Collapse in={expanded} unmountOnExit>
        {node.children.map((child) => (
          <FileTreeNode
            key={child.path}
            node={child}
            depth={depth + 1}
            selectedPath={selectedPath}
            disabledPaths={disabledPaths}
            initiallyExpanded={initiallyExpanded}
            onSelect={onSelect}
          />
        ))}
      </Collapse>
    </Box>
  )
}

// Pre-expand directories that contain at least one file, so the user
// doesn't have to drill down through `.goose/recipes/` on first open.
const collectAutoExpand = (node: TreeNode, acc: Set<string>): boolean => {
  if (node.isFile) return true
  const hasFile = node.children
    .map((c) => collectAutoExpand(c, acc))
    .some(Boolean)
  if (hasFile && node.path) acc.add(node.path)
  return hasFile
}

interface GooseRecipesEditorProps {
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
    queryKey: ['app-goose-recipe-candidates', appId, recipeRepoURL],
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
  const repoOptions = candidatesResponse?.repositories || []
  const projectId = candidatesResponse?.project_id || ''
  const orgId = candidatesResponse?.org_id || ''
  const candidatesError = candidatesResponse?.error || ''

  const tree = useMemo(() => buildTree(candidates), [candidates])
  const usedPaths = useMemo(
    () => new Set(recipes.map((r) => r.path)),
    [recipes],
  )
  const autoExpand = useMemo(() => {
    const acc = new Set<string>()
    collectAutoExpand(tree, acc)
    return acc
  }, [tree])

  const derivedName = useMemo(() => deriveRecipeName(draftPath), [draftPath])
  const effectiveName = (draftNameOverride.trim() || derivedName).trim()

  const draftNameValid = effectiveName === '' || SLASH_COMMAND_PATTERN.test(effectiveName)
  const duplicateName = recipes.some((r) => r.name === effectiveName)

  const canAdd =
    !disabled &&
    draftPath.trim() !== '' &&
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

  const manageReposURL = projectId && orgId
    ? `/orgs/${orgId}/projects/${projectId}/specs?dialog=project-settings&dialog_project_id=${projectId}&dialog_project_settings_tab=repositories`
    : ''

  return (
    <Box>
      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
        Goose Recipes
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
        Recipes become a dropdown on the spec-task form and slash commands
        inside the Goose thread. Pick the repo that hosts your recipe YAML
        files, then pick a file from the tree.
      </Typography>

      <Stack direction="row" spacing={1} alignItems="flex-end" sx={{ mb: 2 }}>
        <FormControl size="small" sx={{ flex: 1 }} disabled={disabled}>
          <InputLabel id="goose-recipe-repo-label">Recipe repository</InputLabel>
          <Select
            labelId="goose-recipe-repo-label"
            label="Recipe repository"
            value={recipeRepoURL}
            onChange={(e) => onChange({ recipeRepoURL: e.target.value, recipes })}
          >
            <MenuItem value="">
              <em>(Use project's primary repository)</em>
            </MenuItem>
            {repoOptions.map((opt) => (
              <MenuItem key={opt.url} value={opt.url}>
                {opt.name}
                {opt.is_primary && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ ml: 1 }}
                  >
                    (primary)
                  </Typography>
                )}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        {manageReposURL && (
          <Link
            href={manageReposURL}
            underline="hover"
            variant="caption"
            sx={{ pb: 1, whiteSpace: 'nowrap' }}
          >
            Manage repositories →
          </Link>
        )}
      </Stack>

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

      <Box
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
          maxHeight: 320,
          overflowY: 'auto',
          mb: 1.5,
        }}
      >
        {loadingCandidates ? (
          <Typography variant="caption" color="text.secondary" sx={{ p: 2, display: 'block' }}>
            Loading recipe files…
          </Typography>
        ) : tree.children.length === 0 ? (
          <Typography variant="caption" color="text.secondary" sx={{ p: 2, display: 'block' }}>
            No YAML files found in this repository. Add a recipe file in your
            repo (typically under <code>.goose/recipes/</code>) and refresh.
          </Typography>
        ) : (
          <Box sx={{ py: 0.5 }}>
            {tree.children.map((child) => (
              <FileTreeNode
                key={child.path}
                node={child}
                depth={0}
                selectedPath={draftPath}
                disabledPaths={usedPaths}
                initiallyExpanded={autoExpand}
                onSelect={setDraftPath}
              />
            ))}
          </Box>
        )}
      </Box>

      <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <TextField
          label="Selected file"
          value={draftPath}
          size="small"
          placeholder="Pick a file from the tree above"
          disabled
          sx={{ flex: 2, minWidth: 280, '& .MuiInputBase-input': { fontFamily: 'monospace' } }}
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
          sx={{ minWidth: 200, flex: 1 }}
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
