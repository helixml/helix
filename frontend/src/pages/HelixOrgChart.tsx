import { FC, useCallback, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Divider from '@mui/material/Divider'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import CloseIcon from '@mui/icons-material/Close'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import dagre from 'dagre'
import {
  Background,
  Controls,
  Edge,
  Handle,
  MiniMap,
  Node,
  NodeProps,
  Position as RFPosition,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  ChartNode,
  HireWorkerRequest,
  WorkerBadge,
  useFireHelixOrgWorker,
  useHelixOrgChart,
  useHelixOrgWorker,
  useHireHelixOrgWorker,
} from '../services/helixOrgService'

// React Flow chart for the helix-org tree. Each Position is a node;
// edges link parent → child. Workers render as inline chips inside
// each Position node. Clicking a Position opens the hire side-panel;
// clicking a Worker chip opens the worker side-panel with the fire
// action + link into the existing detail editor.
//
// Layout is computed by dagre (top-to-bottom) over the Position tree;
// dagre runs on a flattened ChartNode list rather than the raw tree
// so each Position gets exactly one layout pass independent of depth.

const NODE_WIDTH = 280
const ROW_HEIGHT_BASE = 96
const ROW_HEIGHT_PER_WORKER_BLOCK = 28

type FlatPosition = {
  position_id: string
  role_id: string
  parent_id?: string
  workers: WorkerBadge[]
}

type PositionNodeData = FlatPosition & {
  onHire: (positionId: string) => void
  onSelectWorker: (workerId: string) => void
  selectedWorkerId?: string
}

type Selection =
  | { kind: 'none' }
  | { kind: 'position'; positionId: string }
  | { kind: 'worker'; workerId: string }

const flattenChart = (roots: ChartNode[]): FlatPosition[] => {
  const out: FlatPosition[] = []
  const walk = (node: ChartNode, parentId?: string) => {
    out.push({
      position_id: node.position_id,
      role_id: node.role_id,
      parent_id: parentId,
      workers: node.workers ?? [],
    })
    for (const child of node.children ?? []) {
      walk(child, node.position_id)
    }
  }
  for (const root of roots) walk(root)
  return out
}

const estimateNodeHeight = (workerCount: number): number => {
  if (workerCount === 0) return ROW_HEIGHT_BASE
  return ROW_HEIGHT_BASE + Math.ceil(workerCount / 2) * ROW_HEIGHT_PER_WORKER_BLOCK
}

// layout runs dagre over the flat position list and returns nodes
// positioned absolutely. dagre's coordinates are node-centred; React
// Flow expects top-left, so we shift each by half its size.
const layout = (
  flat: FlatPosition[],
  selectedWorkerId: string | undefined,
  onHire: (positionId: string) => void,
  onSelectWorker: (workerId: string) => void,
): { nodes: Node<PositionNodeData>[]; edges: Edge[] } => {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'TB', nodesep: 60, ranksep: 80, marginx: 24, marginy: 24 })
  g.setDefaultEdgeLabel(() => ({}))

  for (const p of flat) {
    g.setNode(p.position_id, { width: NODE_WIDTH, height: estimateNodeHeight(p.workers.length) })
  }
  for (const p of flat) {
    if (p.parent_id) {
      g.setEdge(p.parent_id, p.position_id)
    }
  }
  dagre.layout(g)

  const nodes: Node<PositionNodeData>[] = flat.map((p) => {
    const layoutNode = g.node(p.position_id)
    const height = estimateNodeHeight(p.workers.length)
    return {
      id: p.position_id,
      type: 'position',
      position: {
        x: layoutNode.x - NODE_WIDTH / 2,
        y: layoutNode.y - height / 2,
      },
      data: {
        ...p,
        onHire,
        onSelectWorker,
        selectedWorkerId,
      },
      width: NODE_WIDTH,
      height,
    }
  })

  const edges: Edge[] = flat
    .filter((p) => p.parent_id)
    .map((p) => ({
      id: `${p.parent_id}->${p.position_id}`,
      source: p.parent_id!,
      target: p.position_id,
      type: 'smoothstep',
      animated: false,
      style: { stroke: 'rgba(255,255,255,0.25)', strokeWidth: 1.5 },
    }))

  return { nodes, edges }
}

// PositionNode is the custom React Flow node. The outer Box catches
// the node-selection click; worker chips and the hire button
// stopPropagation so they don't double as a position click.
const PositionNode: FC<NodeProps<Node<PositionNodeData>>> = ({ data, selected }) => {
  return (
    <Box
      sx={{
        width: NODE_WIDTH,
        border: '1px solid',
        borderColor: selected ? 'primary.main' : 'rgba(255,255,255,0.16)',
        borderRadius: 1.5,
        backgroundColor: selected ? 'rgba(33,150,243,0.05)' : 'rgba(255,255,255,0.03)',
        boxShadow: selected ? '0 0 0 1px rgba(33,150,243,0.4)' : 'none',
        transition: 'all 120ms ease',
      }}
    >
      <Handle type="target" position={RFPosition.Top} style={{ background: 'rgba(255,255,255,0.35)' }} />

      <Box sx={{ p: 1.5 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace', fontSize: '0.65rem' }}>
            {data.position_id}
          </Typography>
          <Tooltip title="Hire a worker into this position">
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                data.onHire(data.position_id)
              }}
              sx={{ p: 0.25 }}
            >
              <AddIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        </Stack>

        <Typography variant="body2" sx={{ mt: 0.25, fontWeight: 600 }}>
          {data.role_id}
        </Typography>

        {data.workers.length > 0 && (
          <Stack direction="row" sx={{ mt: 1.25, flexWrap: 'wrap', gap: 0.5 }}>
            {data.workers.map((w) => {
              const isAi = w.kind === 'ai'
              const isSelected = data.selectedWorkerId === w.id
              return (
                <Chip
                  key={w.id}
                  icon={isAi ? <SmartToyOutlinedIcon /> : <PersonOutlineIcon />}
                  label={w.id}
                  size="small"
                  variant={isSelected ? 'filled' : 'outlined'}
                  color={isSelected ? 'primary' : 'default'}
                  onClick={(e) => {
                    e.stopPropagation()
                    data.onSelectWorker(w.id)
                  }}
                  sx={{ fontFamily: 'monospace', fontSize: '0.65rem', height: 22 }}
                />
              )
            })}
          </Stack>
        )}

        {data.workers.length === 0 && (
          <Typography variant="caption" sx={{ display: 'block', mt: 1, color: 'text.secondary', fontStyle: 'italic' }}>
            no workers — click + to hire
          </Typography>
        )}
      </Box>

      <Handle type="source" position={RFPosition.Bottom} style={{ background: 'rgba(255,255,255,0.35)' }} />
    </Box>
  )
}

const nodeTypes = { position: PositionNode }

// HireDrawer is the right-side panel shown when a position is
// selected (or its + button is clicked). On submit it calls
// useHireHelixOrgWorker, which invalidates chart + workers queries.
const HireDrawer: FC<{
  positionId: string
  roleId: string
  onClose: () => void
}> = ({ positionId, roleId, onClose }) => {
  const snackbar = useSnackbar()
  const hire = useHireHelixOrgWorker()
  const [id, setId] = useState('')
  const [kind, setKind] = useState<'ai' | 'human'>('ai')
  const [identity, setIdentity] = useState('')

  const submit = async () => {
    if (!identity.trim()) {
      snackbar.error('identity content is required')
      return
    }
    const body: HireWorkerRequest = {
      position_id: positionId,
      kind,
      identity_content: identity,
    }
    if (id.trim()) body.id = id.trim()
    try {
      const res = await hire.mutateAsync(body)
      snackbar.success(`hired ${res.id}`)
      setId('')
      setIdentity('')
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'hire failed')
    }
  }

  return (
    <Box sx={{ p: 2.5, width: 380 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h6">Hire worker</Typography>
        <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
      </Stack>
      <Stack spacing={1.5}>
        <Box>
          <Typography variant="caption" color="text.secondary">Position</Typography>
          <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{positionId}</Typography>
        </Box>
        <Box>
          <Typography variant="caption" color="text.secondary">Role</Typography>
          <Typography variant="body2">{roleId}</Typography>
        </Box>
        <Divider sx={{ my: 1 }} />
        <TextField
          select
          size="small"
          label="Kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as 'ai' | 'human')}
          fullWidth
        >
          <MenuItem value="ai">AI</MenuItem>
          <MenuItem value="human">Human</MenuItem>
        </TextField>
        <TextField
          size="small"
          label="Handle (optional)"
          placeholder="w-mark"
          helperText="lowercase first name, prefixed with w-. Leave blank to auto-assign."
          value={id}
          onChange={(e) => setId(e.target.value)}
          fullWidth
        />
        <TextField
          size="small"
          label="Identity content"
          placeholder="Short persona / profile in markdown. Projected into the worker's identity.md at activation."
          value={identity}
          onChange={(e) => setIdentity(e.target.value)}
          multiline
          minRows={6}
          fullWidth
        />
        <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
          <Button variant="contained" onClick={submit} disabled={hire.isPending}>
            {hire.isPending ? 'Hiring…' : 'Hire'}
          </Button>
          <Button variant="text" onClick={onClose}>Cancel</Button>
        </Stack>
      </Stack>
    </Box>
  )
}

const WorkerDrawer: FC<{
  workerId: string
  onClose: () => void
}> = ({ workerId, onClose }) => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const { data, isLoading } = useHelixOrgWorker(workerId)
  const fire = useFireHelixOrgWorker()
  const [confirming, setConfirming] = useState(false)

  const fireWorker = async () => {
    try {
      await fire.mutateAsync(workerId)
      snackbar.success(`fired ${workerId}`)
      onClose()
    } catch (err: any) {
      const status = err?.response?.status
      const msg = err?.response?.data?.error ?? err?.message ?? 'fire failed'
      if (status === 409) {
        snackbar.error('owner worker is protected and cannot be fired')
      } else {
        snackbar.error(msg)
      }
    }
  }

  return (
    <Box sx={{ p: 2.5, width: 380 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h6">Worker</Typography>
        <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
      </Stack>
      {isLoading || !data ? (
        <LoadingSpinner />
      ) : (
        <Stack spacing={1.5}>
          <Box>
            <Typography variant="caption" color="text.secondary">ID</Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.worker.id}</Typography>
          </Box>
          <Stack direction="row" spacing={1.5}>
            <Box>
              <Typography variant="caption" color="text.secondary">Kind</Typography>
              <Typography variant="body2">{data.worker.kind}</Typography>
            </Box>
            {data.position && (
              <Box>
                <Typography variant="caption" color="text.secondary">Position</Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.position.id}</Typography>
              </Box>
            )}
            {data.role && (
              <Box>
                <Typography variant="caption" color="text.secondary">Role</Typography>
                <Typography variant="body2">{data.role.id}</Typography>
              </Box>
            )}
          </Stack>

          {(data.worker.tools ?? []).length > 0 && (
            <Box>
              <Typography variant="caption" color="text.secondary">Tools</Typography>
              <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
                {data.worker.tools!.map((t) => (
                  <Chip key={t} size="small" label={t} sx={{ fontSize: '0.65rem', height: 20 }} />
                ))}
              </Stack>
            </Box>
          )}

          <Box>
            <Typography variant="caption" color="text.secondary">Identity (preview)</Typography>
            <Box
              sx={{
                mt: 0.5,
                p: 1,
                maxHeight: 200,
                overflow: 'auto',
                fontSize: '0.75rem',
                fontFamily: 'monospace',
                whiteSpace: 'pre-wrap',
                backgroundColor: 'rgba(0,0,0,0.2)',
                borderRadius: 1,
              }}
            >
              {data.worker.identity_content || '(empty)'}
            </Box>
          </Box>

          <Divider sx={{ my: 1 }} />

          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              size="small"
              startIcon={<OpenInNewIcon />}
              onClick={() => router.navigate('helix_org_worker_detail', { worker_id: workerId })}
            >
              Open editor
            </Button>
            {confirming ? (
              <>
                <Button
                  variant="contained"
                  color="error"
                  size="small"
                  startIcon={<DeleteOutlineIcon />}
                  onClick={fireWorker}
                  disabled={fire.isPending}
                >
                  {fire.isPending ? 'Firing…' : 'Confirm fire'}
                </Button>
                <Button size="small" onClick={() => setConfirming(false)}>Cancel</Button>
              </>
            ) : (
              <Button
                variant="outlined"
                color="error"
                size="small"
                startIcon={<DeleteOutlineIcon />}
                onClick={() => setConfirming(true)}
              >
                Fire
              </Button>
            )}
          </Stack>
        </Stack>
      )}
    </Box>
  )
}

// ChartCanvas wraps ReactFlow so we can use the useReactFlow hook to
// fitView whenever the layout regenerates (e.g. after a hire).
const ChartCanvas: FC<{
  flat: FlatPosition[]
  selection: Selection
  setSelection: (sel: Selection) => void
}> = ({ flat, selection, setSelection }) => {
  const { fitView } = useReactFlow()

  const selectedWorkerId = selection.kind === 'worker' ? selection.workerId : undefined

  const openHire = useCallback(
    (positionId: string) => {
      setSelection({ kind: 'position', positionId })
    },
    [setSelection],
  )
  const openWorker = useCallback(
    (workerId: string) => {
      setSelection({ kind: 'worker', workerId })
    },
    [setSelection],
  )

  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => layout(flat, selectedWorkerId, openHire, openWorker),
    [flat, selectedWorkerId, openHire, openWorker],
  )

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  useEffect(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
    // Defer fitView until the next frame so React Flow has the new
    // nodes/edges committed.
    requestAnimationFrame(() => fitView({ padding: 0.2, duration: 250 }))
  }, [initialNodes, initialEdges, fitView, setNodes, setEdges])

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.2 }}
      proOptions={{ hideAttribution: true }}
      colorMode="dark"
      onNodeClick={(_, node) => setSelection({ kind: 'position', positionId: node.id })}
      onPaneClick={() => setSelection({ kind: 'none' })}
    >
      <Background gap={20} size={1} />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable maskColor="rgba(0,0,0,0.6)" />
    </ReactFlow>
  )
}

const HelixOrgChart: FC = () => {
  const { data, isLoading } = useHelixOrgChart()
  const roots = data?.roots ?? []
  const flat = useMemo(() => flattenChart(roots), [roots])

  const [selection, setSelection] = useState<Selection>({ kind: 'none' })

  const selectedPosition = useMemo(() => {
    if (selection.kind !== 'position') return undefined
    return flat.find((p) => p.position_id === selection.positionId)
  }, [selection, flat])

  return (
    <Page breadcrumbTitle="Org Chart" breadcrumbParent={{ title: 'Helix Org' }}>
      <Box sx={{ position: 'absolute', inset: 0, top: 64, display: 'flex', flexDirection: 'column' }}>
        <Box sx={{ px: 3, py: 2 }}>
          <Typography variant="h5">Org Chart</Typography>
          <Typography variant="body2" color="text.secondary">
            Positions form the tree. Click <AddIcon sx={{ fontSize: 14, verticalAlign: 'middle' }} /> on a position to hire a worker, or click a worker chip to inspect / fire.
          </Typography>
        </Box>
        <Box sx={{ flex: 1, position: 'relative', minHeight: 0 }}>
          {isLoading ? (
            <Box sx={{ p: 4 }}><LoadingSpinner /></Box>
          ) : flat.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 6 }}>
              <Typography variant="body1" color="text.secondary">
                No positions yet. Bootstrap creates the owner position automatically — restart the API if you don't see it.
              </Typography>
            </Box>
          ) : (
            <ReactFlowProvider>
              <ChartCanvas flat={flat} selection={selection} setSelection={setSelection} />
            </ReactFlowProvider>
          )}
        </Box>
      </Box>

      <Drawer
        anchor="right"
        open={selection.kind !== 'none'}
        onClose={() => setSelection({ kind: 'none' })}
        PaperProps={{ sx: { backgroundImage: 'none' } }}
      >
        {selection.kind === 'position' && selectedPosition && (
          <HireDrawer
            positionId={selectedPosition.position_id}
            roleId={selectedPosition.role_id}
            onClose={() => setSelection({ kind: 'none' })}
          />
        )}
        {selection.kind === 'worker' && (
          <WorkerDrawer
            workerId={selection.workerId}
            onClose={() => setSelection({ kind: 'none' })}
          />
        )}
      </Drawer>
    </Page>
  )
}

export default HelixOrgChart
