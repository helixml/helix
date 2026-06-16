// HelixOrgRoleDetail edits a single role end-to-end: markdown content
// and tools (the role's MCP tool list). Lives at
// `/orgs/:org_id/helix-org/roles/:role_id` and is the destination
// both the chart's role drawer and the Roles list link to.
//
// Stream subscriptions are NOT edited here — subscriptions are
// worker-anchored (they live on the Worker, die when it's fired, and
// aren't inherited by a new hire into the same Role), so they're
// managed on the Worker detail page, not the Role.
//
// Markdown editing uses the in-tree Monaco editor (loaded everywhere
// else via `components/widgets/MonacoEditor`). Tools are edited via a
// multi-select over the tool catalogue.

import { FC, Key, useEffect, useMemo, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Grid from '@mui/material/Grid'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CheckBoxIcon from '@mui/icons-material/CheckBox'
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import SaveIcon from '@mui/icons-material/Save'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import MonacoEditor from '../components/widgets/MonacoEditor'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  ToolDTO,
  useDeleteHelixOrgRole,
  useHelixOrgRole,
  useListHelixOrgTools,
  useUpdateHelixOrgRole,
} from '../services/helixOrgService'

const HelixOrgRoleDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const orgSlug = router.params.org_id as string | undefined
  const roleId = router.params.role_id as string | undefined
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Roles', routeName: 'helix_org_roles' })

  const { data, isLoading } = useHelixOrgRole(roleId)
  const { data: toolCatalogue } = useListHelixOrgTools()
  const updateRole = useUpdateHelixOrgRole()
  const deleteRole = useDeleteHelixOrgRole()

  const [content, setContent] = useState('')
  const [tools, setTools] = useState<string[]>([])
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  // Seed local state when the role loads or the route changes.
  useEffect(() => {
    if (!data) return
    setContent(data.content ?? '')
    setTools(data.tools ?? [])
  }, [data])

  // The Autocomplete needs Option objects, but the role's tool list
  // is just a string[] of names. We render every catalogue
  // entry plus any role-listed names the catalogue didn't return
  // (defensive — if a tool was unregistered but the role still
  // lists it, we keep showing it as selected so the operator can
  // explicitly remove it).
  const toolOptions = useMemo<ToolDTO[]>(() => {
    const cat = toolCatalogue ?? []
    const known = new Set(cat.map((t) => t.name))
    const extras = tools
      .filter((name) => !known.has(name))
      .map<ToolDTO>((name) => ({ name, description: '(not in current catalogue)' }))
    return [...cat, ...extras]
  }, [toolCatalogue, tools])

  const dirty = useMemo(() => {
    if (!data) return false
    if ((data.content ?? '') !== content) return true
    if ((data.tools ?? []).join(',') !== tools.join(',')) return true
    return false
  }, [data, content, tools])

  const handleSave = async () => {
    if (!roleId) return
    try {
      // Streams are intentionally omitted — they're worker-anchored and
      // managed on the Worker detail page. The backend preserves a
      // Role's existing streams when the field is absent.
      await updateRole.mutateAsync({
        id: roleId,
        content,
        tools,
      })
      snackbar.success(`role ${roleId} saved`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'save failed')
    }
  }

  const handleDelete = async () => {
    if (!roleId) return
    try {
      await deleteRole.mutateAsync(roleId)
      snackbar.success(`deleted role ${roleId}`)
      if (orgSlug) {
        router.navigate('helix_org_roles', { org_id: orgSlug })
      }
    } catch (err: any) {
      const status = err?.response?.status
      if (status === 409) {
        snackbar.error('owner role is protected and cannot be deleted')
      } else {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'delete failed')
      }
    } finally {
      setConfirmingDelete(false)
    }
  }

  return (
    <Page
      breadcrumbTitle={roleId ?? 'Role'}
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <Stack direction="row" spacing={1}>
          <Button
            variant="contained"
            color="secondary"
            startIcon={<SaveIcon />}
            disabled={!dirty || updateRole.isPending}
            onClick={handleSave}
          >
            {updateRole.isPending ? 'Saving…' : 'Save'}
          </Button>
        </Stack>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        {isLoading || !data ? (
          <LoadingSpinner />
        ) : (
          <Grid container spacing={3}>
            {/* Main editor column */}
            <Grid item xs={12} md={9}>
              <Stack spacing={3}>
                <Box>
                  <Typography variant="h5" sx={{ fontFamily: 'monospace', mb: 0.5 }}>
                    {data.id}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    The role's markdown is the job description every Worker in this role reads on
                    activation. Cmd/Ctrl+S inside the editor saves.
                  </Typography>
                </Box>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Content (markdown)</Typography>
                  <MonacoEditor
                    value={content}
                    onChange={setContent}
                    onSave={handleSave}
                    language="markdown"
                    minHeight={320}
                    maxHeight={720}
                    autoHeight={true}
                    theme="helix-dark"
                  />
                </Box>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Tools</Typography>
                  <Autocomplete
                    multiple
                    disableCloseOnSelect
                    options={toolOptions}
                    value={toolOptions.filter((o) => tools.includes(o.name))}
                    onChange={(_e, value) => setTools(value.map((v) => v.name))}
                    getOptionLabel={(o) => o.name}
                    isOptionEqualToValue={(a, b) => a.name === b.name}
                    renderOption={(props, option, { selected }) => {
                      // Pass key explicitly rather than via the props
                      // spread — React 18.3 warns when a spread object
                      // carries a key.
                      const { key, ...liProps } = props as typeof props & { key?: Key }
                      return (
                        <li key={key ?? option.name} {...liProps}>
                          <Checkbox
                            icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
                            checkedIcon={<CheckBoxIcon fontSize="small" />}
                            style={{ marginRight: 8 }}
                            checked={selected}
                          />
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                              {option.name}
                            </Typography>
                            {option.description && (
                              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                {option.description}
                              </Typography>
                            )}
                          </Box>
                        </li>
                      )
                    }}
                    renderTags={(value, getTagProps) =>
                      value.map((option, index) => {
                        const { key, ...tagProps } = getTagProps({ index })
                        return (
                          <Chip
                            key={key ?? option.name}
                            {...tagProps}
                            label={option.name}
                            size="small"
                            sx={{ fontFamily: 'monospace' }}
                          />
                        )
                      })
                    }
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        placeholder={tools.length === 0 ? 'Pick the tools for workers in this role' : ''}
                        helperText="MCP tools the Workers in this role can call. Empty = no tools (workers can still receive owner-chat)."
                      />
                    )}
                  />
                </Box>

              </Stack>
            </Grid>

            {/* Right rail: high-level actions + audit */}
            <Grid item xs={12} md={3}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={2}>
                  <Box>
                    <Typography variant="caption" color="text.secondary">ID</Typography>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.id}</Typography>
                  </Box>
                  {data.created_at && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Created</Typography>
                      <Typography variant="body2">{new Date(data.created_at).toLocaleString()}</Typography>
                    </Box>
                  )}
                  {data.updated_at && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Updated</Typography>
                      <Typography variant="body2">{new Date(data.updated_at).toLocaleString()}</Typography>
                    </Box>
                  )}
                  <Divider />
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteOutlineIcon />}
                    onClick={() => setConfirmingDelete(true)}
                    fullWidth
                  >
                    Delete role
                  </Button>
                  <Typography variant="caption" color="text.secondary">
                    Fires every Worker holding this Role and drops their subscriptions.
                  </Typography>
                </Stack>
              </Paper>
            </Grid>
          </Grid>
        )}
      </Container>

      {confirmingDelete && roleId && (
        <DeleteConfirmWindow
          title="role"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setConfirmingDelete(false)}
        >
          <Typography variant="body1">
            Deleting role <b style={{ fontFamily: 'monospace' }}>{roleId}</b> cascades:
            every position under it is deleted and every worker in those positions is fired.
            This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
    </Page>
  )
}

export default HelixOrgRoleDetail
