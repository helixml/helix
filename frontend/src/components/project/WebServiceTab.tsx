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
  const cnameTarget = data?.cname_target ?? ''

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
          Host this project as a live website. Turn it on and you'll get
          a default URL straight away. You can also point your own domain
          at it. Every push to your default branch deploys automatically —
          Helix runs the <code>.helix/startup.sh</code> script from your
          repo's <code>helix-specs</code> branch to start your app.
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
            <Typography variant="subtitle1" gutterBottom>
              Storage &amp; runner
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Durable data lives at <code>/data</code> inside the container —
              write your database, uploads and other persistent files there
              (your <code>.helix/startup.sh</code> receives the path as
              <code>$HELIX_WEB_SERVICE_DATA_DIR</code>). It survives redeploys
              and reboots.
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Pinned runner:{' '}
              <code>{state?.host_device_id || 'assigned on first deploy'}</code>.
              The service is pinned to this runner so its data stays reachable.
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Deploys restart the app in place (one instance at a time, so a
              database is never opened by two processes), which causes a brief
              restart window of downtime. For zero-downtime blue/green or
              scaling, deploy to an external Kubernetes cluster instead.
            </Typography>
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
              cnameTarget={cnameTarget}
              onDelete={(id) => deleteDomainMutation.mutate(id)}
              onCopy={(text, label) => {
                navigator.clipboard.writeText(text)
                snackbar.success(`${label} copied`)
              }}
            />

            <Stack direction="row" spacing={1} mt={2}>
              <TextField
                size="small"
                placeholder="app.yourcompany.com"
                value={newDomain}
                onChange={(e) => setNewDomain(e.target.value)}
                sx={{ flex: 1 }}
              />
              <Button
                variant="contained"
                size="small"
                disabled={!newDomain || addDomainMutation.isPending}
                onClick={() => addDomainMutation.mutate(newDomain.trim())}
              >
                Add domain
              </Button>
            </Stack>
            {cnameTarget && (
              <Alert severity="info" sx={{ mt: 2 }}>
                <Typography variant="body2" sx={{ mb: 1, fontWeight: 600 }}>
                  How to add a custom domain
                </Typography>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  1. Type your domain above (for example,{' '}
                  <code>app.yourcompany.com</code>) and click <strong>Add
                  domain</strong>.
                </Typography>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  2. In your DNS provider (e.g. Cloudflare, Route 53,
                  Namecheap), add a <strong>CNAME</strong> record:
                </Typography>
                <Box
                  sx={{
                    fontFamily: 'monospace',
                    fontSize: '0.85rem',
                    p: 1.5,
                    my: 1,
                    borderRadius: 1,
                    backgroundColor: 'rgba(0,0,0,0.15)',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
                    flexWrap: 'wrap',
                  }}
                >
                  <span>
                    <strong>Name:</strong> app (or whatever you chose)
                  </span>
                  <span>
                    <strong>Type:</strong> CNAME
                  </span>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                    <strong>Value:</strong> {cnameTarget}
                    <IconButton
                      size="small"
                      onClick={() => {
                        navigator.clipboard.writeText(cnameTarget)
                        snackbar.success('CNAME target copied')
                      }}
                    >
                      <ContentCopyIcon fontSize="small" />
                    </IconButton>
                  </span>
                </Box>
                <Typography variant="body2">
                  3. That's it. Helix checks every minute and your domain
                  will go live automatically — usually within a couple
                  of minutes once DNS has propagated. The HTTPS certificate
                  is issued and renewed for you, no extra steps.
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ mt: 1.5, pt: 1.5, borderTop: '1px solid rgba(0,0,0,0.12)' }}
                >
                  <strong>Behind Cloudflare or another proxy/CDN?</strong> Point the proxy
                  straight at <code>{cnameTarget}</code> and it still works. Because a proxy
                  hides your server from Let's Encrypt, the certificate for the Helix ↔ proxy
                  connection needs a one-time <strong>ACME challenge delegation</strong> — get
                  in touch and we'll give you the exact <code>_acme-challenge</code> record to add.
                  Domains pointed directly at <code>{cnameTarget}</code> (no proxy) need none of this.
                </Typography>
              </Alert>
            )}
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
            <DeploysTable deploys={deploys} hasActiveSandbox={!!state?.active_sandbox_id} />
          </Box>
        </>
      )}
    </Stack>
  )
}

const DomainList: FC<{
  domains: TypesVHostRoute[]
  cnameTarget: string
  onDelete: (id: string) => void
  onCopy: (text: string, label: string) => void
}> = ({ domains, cnameTarget, onDelete, onCopy }) => {
  if (domains.length === 0) {
    return <Typography variant="body2" color="text.secondary">No domains yet.</Typography>
  }
  return (
    <Stack spacing={1}>
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
                  <Chip size="small" color="success" label="Live" />
                ) : (
                  <Chip size="small" color="warning" label="Waiting for DNS" />
                )}
              </TableCell>
              <TableCell align="right">
                {d.verified_at && (
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
                )}
                <Tooltip title="Copy URL">
                  <IconButton
                    size="small"
                    onClick={() => onCopy(`https://${d.hostname}/`, 'URL')}
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

      {/* Inline "what to do next" prompt for any pending custom domain */}
      {domains
        .filter((d) => !d.verified_at && !d.is_default)
        .map((d) => (
          <Alert key={`pending-${d.id}`} severity="warning" icon={false}>
            <Typography variant="body2" sx={{ mb: 1 }}>
              <strong>{d.hostname}</strong> is waiting for DNS. Add this
              CNAME record at your DNS provider, then we'll take it from
              there:
            </Typography>
            <Box
              sx={{
                fontFamily: 'monospace',
                fontSize: '0.85rem',
                p: 1.5,
                borderRadius: 1,
                backgroundColor: 'rgba(0,0,0,0.15)',
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                flexWrap: 'wrap',
              }}
            >
              <span><strong>Name:</strong> {leftmostLabel(d.hostname)}</span>
              <span><strong>Type:</strong> CNAME</span>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                <strong>Value:</strong> {cnameTarget || '(operator has not configured a canonical hostname)'}
                {cnameTarget && (
                  <IconButton size="small" onClick={() => onCopy(cnameTarget, 'CNAME target')}>
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                )}
              </span>
            </Box>
          </Alert>
        ))}
    </Stack>
  )
}

// leftmostLabel returns just the first DNS label of a hostname — that's
// what the user types as the "Name" field in their DNS provider's UI
// (most providers auto-append the zone).
const leftmostLabel = (hostname: string): string => {
  const parts = hostname.split('.')
  return parts[0] || hostname
}

const ActiveSandboxSummary: FC<{ state?: TypesProjectWebServiceState }> = ({ state }) => {
  if (!state?.active_sandbox_id) {
    return (
      <Typography variant="body2" color="text.secondary">
        Nothing's been deployed yet. Push to your default branch (or click
        Deploy now) to launch your app.
      </Typography>
    )
  }
  return (
    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
      {state.active_sandbox_id}
    </Typography>
  )
}

const DeploysTable: FC<{
  deploys: TypesWebServiceDeploy[]
  hasActiveSandbox: boolean
}> = ({ deploys, hasActiveSandbox }) => {
  if (deploys.length === 0) {
    if (hasActiveSandbox) {
      // Active sandbox without a deploy row — pre-existing state, e.g.
      // an operator wired things up by hand. Don't make the UI look broken.
      return (
        <Typography variant="body2" color="text.secondary">
          No deploy history. The active sandbox above was set up directly
          (no deploy was recorded). Future deploys will appear here.
        </Typography>
      )
    }
    return (
      <Typography variant="body2" color="text.secondary">
        No deploys yet. Your first push (or Deploy now) will appear here.
      </Typography>
    )
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
