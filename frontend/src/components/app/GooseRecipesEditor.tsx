import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import Collapse from '@mui/material/Collapse'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import ListSubheader from '@mui/material/ListSubheader'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import FolderIcon from '@mui/icons-material/Folder'
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined'
import LinkIcon from '@mui/icons-material/Link'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useAttachRepositoryToProject } from '../../services'
import { useGitRepositories } from '../../services/gitRepositoryService'
import { IAssistantGooseRecipe } from '../../types'
import { ServerGooseRecipeCandidate } from '../../api/api'

// Sentinel value used in the dropdown to mean "open the attach dialog".
// MUI Select uses string-equality on values, so any out-of-band token
// (one that can't collide with a real URL) works — pick a clearly
// invalid scheme so it's obvious in logs.
const ATTACH_NEW_REPO_SENTINEL = '__helix__:attach-new-repo'

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

  // Inline attach-repo dialog state. Reuses the same mutation as the
  // Project Settings → Repositories tab so attach + cache invalidation
  // behave identically — picking "+ Attach a new repository…" should
  // feel like a shortcut, not a separate code path.
  const account = useAccount()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const [attachDialogOpen, setAttachDialogOpen] = useState(false)
  const [selectedRepoToAttach, setSelectedRepoToAttach] = useState('')
  const currentOrgID = account.organizationTools.organization?.id
  const { data: allUserRepositories = [] } = useGitRepositories(
    currentOrgID
      ? { organizationId: currentOrgID }
      : account.user?.id
        ? { ownerId: account.user.id }
        : { enabled: false },
  )
  const attachRepoMutation = useAttachRepositoryToProject(projectId)

  // Repos eligible for attaching: belong to the user/org but not yet
  // attached to this project. Matches the same filter used in
  // ProjectSettings.tsx so the lists stay consistent.
  const attachableRepos = useMemo(
    () =>
      allUserRepositories.filter(
        (repo: { id?: string }) =>
          repo.id && !repoOptions.some((opt) => opt.url && opt.url === (repo as { external_url?: string; clone_url?: string }).external_url) &&
          !repoOptions.some((opt) => opt.url && opt.url === (repo as { external_url?: string; clone_url?: string }).clone_url),
      ),
    [allUserRepositories, repoOptions],
  )

  const handleAttachRepo = async () => {
    if (!selectedRepoToAttach || !projectId) return
    try {
      await attachRepoMutation.mutateAsync(selectedRepoToAttach)
      snackbar.success('Repository attached')
      setAttachDialogOpen(false)
      setSelectedRepoToAttach('')
      // The candidates list and repos dropdown both come from the
      // candidates query — invalidate it so the new repo appears.
      queryClient.invalidateQueries({
        queryKey: ['app-goose-recipe-candidates', appId],
      })
    } catch (err) {
      snackbar.error('Failed to attach repository')
    }
  }

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

  // Selecting the primary repo from the dropdown stores empty URL —
  // that's the "fall back to primary" mode the backend already
  // implements. So the dropdown's selected value mirrors that: empty
  // when the user picked primary, or the explicit URL otherwise.
  const primaryRepo = repoOptions.find((r) => r.is_primary)
  const dropdownValue = recipeRepoURL === '' && primaryRepo
    ? primaryRepo.url || ''
    : recipeRepoURL

  const handleRepoSelect = (value: string) => {
    if (value === ATTACH_NEW_REPO_SENTINEL) {
      setAttachDialogOpen(true)
      return
    }
    // Normalize picking the primary repo back to empty URL so the
    // backend continues to use its "fallback to primary" path.
    if (primaryRepo && value === primaryRepo.url) {
      onChange({ recipeRepoURL: '', recipes })
    } else {
      onChange({ recipeRepoURL: value, recipes })
    }
  }

  // Unused but kept for now: org_id is no longer needed since the
  // dialog is inline. Keeping it referenced so TS doesn't complain.
  void orgId

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

      <FormControl size="small" fullWidth disabled={disabled} sx={{ mb: 2 }}>
        <InputLabel id="goose-recipe-repo-label">Recipe repository</InputLabel>
        <Select
          labelId="goose-recipe-repo-label"
          label="Recipe repository"
          value={dropdownValue}
          onChange={(e) => handleRepoSelect(e.target.value)}
          renderValue={(value) => {
            const opt = repoOptions.find((r) => r.url === value)
            if (!opt) return value
            return (
              <Box component="span">
                {opt.name}
                {opt.is_primary && (
                  <Typography
                    component="span"
                    variant="caption"
                    color="text.secondary"
                    sx={{ ml: 1 }}
                  >
                    (primary)
                  </Typography>
                )}
              </Box>
            )
          }}
        >
          {repoOptions.length === 0 && (
            <MenuItem value="" disabled>
              <em>No repositories attached to this project</em>
            </MenuItem>
          )}
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
          {projectId && [
            <ListSubheader key="__attach_subheader__" sx={{ lineHeight: 1.5 }} />,
            <MenuItem key="__attach_new__" value={ATTACH_NEW_REPO_SENTINEL}>
              <AddIcon fontSize="small" sx={{ mr: 1 }} />
              Attach a new repository…
            </MenuItem>,
          ]}
        </Select>
      </FormControl>

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

      <Dialog
        open={attachDialogOpen}
        onClose={() => {
          setAttachDialogOpen(false)
          setSelectedRepoToAttach('')
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <LinkIcon />
            Attach Repository to Project
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Pick a repository from your account to attach to this project so
            it becomes available as a recipe source.
          </Typography>
          <FormControl fullWidth size="small">
            <InputLabel>Select Repository</InputLabel>
            <Select
              value={selectedRepoToAttach}
              onChange={(e) => setSelectedRepoToAttach(e.target.value)}
              label="Select Repository"
            >
              {attachableRepos.map((repo: { id?: string; name?: string }) => (
                <MenuItem key={repo.id} value={repo.id}>
                  {repo.name}
                </MenuItem>
              ))}
            </Select>
            {attachableRepos.length === 0 && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 1, display: 'block' }}
              >
                No more repositories available to attach. Create one in
                Repositories first.
              </Typography>
            )}
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAttachDialogOpen(false)
              setSelectedRepoToAttach('')
            }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleAttachRepo}
            disabled={!selectedRepoToAttach || attachRepoMutation.isPending}
          >
            {attachRepoMutation.isPending ? 'Attaching…' : 'Attach'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default GooseRecipesEditor
