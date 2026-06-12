import React, { FC, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import LaunchIcon from '@mui/icons-material/Launch'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import {
  ServerProjectWebServiceResponse,
  TypesProjectWebServiceState,
  TypesVHostRoute,
  TypesWebServiceDeploy,
} from '../../api/api'

interface WebServiceTabProps {
  projectId: string
}

const WebServiceTab: FC<WebServiceTabProps> = ({ projectId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()

  const stateQueryKey = ['project-web-service', projectId]

  const { data, isLoading, error } = useQuery<ServerProjectWebServiceResponse>({
    queryKey: stateQueryKey,
    enabled: !!projectId,
    queryFn: async () => {
      const res = await apiClient.v1ProjectsWebServiceDetail(projectId)
      return res.data
    },
  })

  const state = data?.state
  const domains = data?.domains ?? []
  const deploys = data?.deploys ?? []

  const [containerPortDraft, setContainerPortDraft] = useState<string>('')
  const [newDomain, setNewDomain] = useState('')

  const invalidate = () => queryClient.invalidateQueries({ queryKey: stateQueryKey })

  const updateMutation = useMutation({
    mutationFn: async (body: { enabled?: boolean; container_port?: number }) => {
      const res = await apiClient.v1ProjectsWebServiceUpdate(projectId, body as any)
      return res.data
    },
    onSuccess: () => invalidate(),
    onError: (e: any) => snackbar.error(`Update failed: ${e?.message ?? e}`),
  })

  const addDomainMutation = useMutation({
    mutationFn: async (hostname: string) => {
      const res = await apiClient.v1ProjectsWebServiceDomainsCreate(projectId, {
        hostname,
      } as any)
      return res.data
    },
    onSuccess: () => {
      setNewDomain('')
      invalidate()
    },
    onError: (e: any) => snackbar.error(`${e?.response?.data ?? e?.message ?? e}`),
  })

  const deleteDomainMutation = useMutation({
    mutationFn: async (domainId: string) => {
      await apiClient.v1ProjectsWebServiceDomainsDelete(projectId, domainId)
    },
    onSuccess: () => invalidate(),
    onError: (e: any) => snackbar.error(`Delete failed: ${e?.message ?? e}`),
  })

  const deployMutation = useMutation({
    mutationFn: async () => {
      const res = await apiClient.v1ProjectsWebServiceDeployCreate(projectId, {} as any)
      return res.data
    },
    onSuccess: () => {
      snackbar.success('Deploy started')
      invalidate()
    },
    onError: (e: any) => snackbar.error(`Deploy failed: ${e?.response?.data ?? e?.message ?? e}`),
  })

  if (isLoading) return <CircularProgress />
  if (error) return <Alert severity="error">Failed to load web service state.</Alert>

  const enabled = !!state?.enabled
  const containerPort = state?.container_port ?? 8080
  const portValue = containerPortDraft || String(containerPort)

  return (
    <Stack spacing={3} sx={{ maxWidth: 900 }}>
      <Box>
        <Typography variant="h5" gutterBottom>
          Web service hosting
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Expose this project as a public web app. On enable, a default
          subdomain is allocated automatically. Pushes to the primary repo's
          default branch auto-deploy by running <code>.helix/startup.sh</code>
          inside a fresh sandbox container.
        </Typography>
      </Box>

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
        <Switch
          checked={enabled}
          onChange={(e) => updateMutation.mutate({ enabled: e.target.checked })}
          disabled={updateMutation.isPending}
        />
        <Typography>{enabled ? 'Enabled' : 'Disabled'}</Typography>
      </Box>

      {enabled && (
        <>
          <Divider />

          <Box>
            <Typography variant="subtitle1" gutterBottom>
              Container port
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              <TextField
                size="small"
                value={portValue}
                onChange={(e) => setContainerPortDraft(e.target.value)}
                sx={{ width: 120 }}
              />
              <Button
                variant="outlined"
                size="small"
                disabled={updateMutation.isPending || !containerPortDraft}
                onClick={() => {
                  const n = parseInt(containerPortDraft, 10)
                  if (!Number.isInteger(n) || n < 1 || n > 65535) {
                    snackbar.error('Port must be 1..65535')
                    return
                  }
                  updateMutation.mutate({ container_port: n })
                  setContainerPortDraft('')
                }}
              >
                Save
              </Button>
              <Typography variant="caption" color="text.secondary">
                Your <code>.helix/startup.sh</code> receives this as
                <code>$HELIX_WEB_SERVICE_PORT</code>.
              </Typography>
            </Stack>
          </Box>

          <Divider />

          <Box>
            <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1}>
              <Typography variant="subtitle1">Domains</Typography>
              <Tooltip title="Trigger a deploy at the current HEAD of the primary repo">
                <span>
                  <Button
                    variant="contained"
                    size="small"
                    disabled={deployMutation.isPending}
                    onClick={() => deployMutation.mutate()}
                  >
                    Deploy now
                  </Button>
                </span>
              </Tooltip>
            </Stack>

            <DomainList
              domains={domains}
              onDelete={(id) => deleteDomainMutation.mutate(id)}
            />

            <Stack direction="row" spacing={1} mt={2}>
              <TextField
                size="small"
                placeholder="app.example.com"
                value={newDomain}
                onChange={(e) => setNewDomain(e.target.value)}
                sx={{ flex: 1 }}
              />
              <Button
                variant="outlined"
                size="small"
                disabled={!newDomain || addDomainMutation.isPending}
                onClick={() => addDomainMutation.mutate(newDomain.trim())}
              >
                Add domain
              </Button>
            </Stack>
            <Typography variant="caption" color="text.secondary" display="block" mt={1}>
              After adding, point your DNS at this Helix instance.
              Verification happens automatically once the
              <code>/.well-known/helix-domain-verify/…</code> endpoint round-trips
              (within ~60 seconds).
            </Typography>
          </Box>

          <Divider />

          <Box>
            <Typography variant="subtitle1" gutterBottom>
              Active sandbox
            </Typography>
            <ActiveSandboxSummary state={state} />
          </Box>

          <Divider />

          <Box>
            <Typography variant="subtitle1" gutterBottom>
              Recent deploys
            </Typography>
            <DeploysTable deploys={deploys} />
          </Box>
        </>
      )}
    </Stack>
  )
}

const DomainList: FC<{ domains: TypesVHostRoute[]; onDelete: (id: string) => void }> = ({
  domains,
  onDelete,
}) => {
  if (domains.length === 0) {
    return <Typography variant="body2" color="text.secondary">No domains yet.</Typography>
  }
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>Hostname</TableCell>
          <TableCell>Type</TableCell>
          <TableCell>Status</TableCell>
          <TableCell align="right">Actions</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {domains.map((d) => (
          <TableRow key={d.id}>
            <TableCell>
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                {d.hostname}
              </Typography>
            </TableCell>
            <TableCell>
              {d.is_default ? (
                <Chip size="small" label="Default" />
              ) : (
                <Chip size="small" label="Custom" variant="outlined" />
              )}
            </TableCell>
            <TableCell>
              {d.verified_at ? (
                <Chip size="small" color="success" label="Verified" />
              ) : (
                <Tooltip title={`Verification token: ${d.verification_token}`}>
                  <Chip size="small" color="warning" label="Pending DNS" />
                </Tooltip>
              )}
            </TableCell>
            <TableCell align="right">
              <Tooltip title="Open">
                <IconButton
                  size="small"
                  href={`https://${d.hostname}/`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <LaunchIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              <Tooltip title="Copy URL">
                <IconButton
                  size="small"
                  onClick={() => navigator.clipboard.writeText(`https://${d.hostname}/`)}
                >
                  <ContentCopyIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              {!d.is_default && (
                <Tooltip title="Remove">
                  <IconButton size="small" onClick={() => onDelete(d.id!)}>
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

const ActiveSandboxSummary: FC<{ state?: TypesProjectWebServiceState }> = ({ state }) => {
  if (!state?.active_sandbox_id) {
    return (
      <Typography variant="body2" color="text.secondary">
        No deploy yet. Push to the primary repo's default branch (or click
        "Deploy now") to spin up a sandbox.
      </Typography>
    )
  }
  return (
    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
      {state.active_sandbox_id}
    </Typography>
  )
}

const DeploysTable: FC<{ deploys: TypesWebServiceDeploy[] }> = ({ deploys }) => {
  if (deploys.length === 0) {
    return <Typography variant="body2" color="text.secondary">No deploys yet.</Typography>
  }
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>When</TableCell>
          <TableCell>Status</TableCell>
          <TableCell>Commit</TableCell>
          <TableCell>Sandbox</TableCell>
          <TableCell>Error</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {deploys.map((d) => (
          <TableRow key={d.id}>
            <TableCell>{d.started_at ? new Date(d.started_at).toLocaleString() : '—'}</TableCell>
            <TableCell>
              <StatusChip status={d.status} />
            </TableCell>
            <TableCell sx={{ fontFamily: 'monospace' }}>{(d.commit_sha || '').slice(0, 8)}</TableCell>
            <TableCell sx={{ fontFamily: 'monospace' }}>{d.sandbox_id ?? '—'}</TableCell>
            <TableCell>
              {d.error ? (
                <Tooltip title={d.error}>
                  <Typography variant="caption" color="error">
                    {d.error.slice(0, 60)}…
                  </Typography>
                </Tooltip>
              ) : (
                '—'
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

const StatusChip: FC<{ status?: string }> = ({ status }) => {
  const color =
    status === 'live'
      ? 'success'
      : status === 'failed'
        ? 'error'
        : status === 'superseded'
          ? 'default'
          : 'warning'
  return <Chip size="small" color={color as any} label={status ?? '—'} />
}

export default WebServiceTab
