import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { TypesSpecTaskProposal } from '../../api/api'
import { useSpecTaskProposals } from '../../hooks/useSpecTaskProposals'
import useSnackbar from '../../hooks/useSnackbar'

interface PendingProposalsPanelProps {
  taskId: string
}

/**
 * Shows all pending proposals for a spec task. The agent creates proposals via
 * the MCP tools (propose_pull_request, propose_spec_task, mark_task_complete);
 * this panel surfaces them so the user can approve / reject in the UI. On
 * decision, the backend dispatches the action AND notifies the agent via a
 * follow-up prompt-template message in the agent's session.
 */
export default function PendingProposalsPanel({ taskId }: PendingProposalsPanelProps) {
  const { pending, isLoading } = useSpecTaskProposals(taskId)

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 1 }}>
        <CircularProgress size={16} />
        <Typography variant="caption">Checking for agent proposals…</Typography>
      </Box>
    )
  }
  if (pending.length === 0) {
    return null
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="overline" sx={{ display: 'block', mb: 1, color: 'text.secondary' }}>
        Agent proposals — awaiting your decision ({pending.length})
      </Typography>
      <Stack spacing={2}>
        {pending.map((p) => (
          <ProposalCard key={p.id} proposal={p} taskId={taskId} />
        ))}
      </Stack>
    </Box>
  )
}

function ProposalCard({ proposal, taskId }: { proposal: TypesSpecTaskProposal; taskId: string }) {
  switch (proposal.kind) {
    case 'pull_request':
      return <PRProposalCard proposal={proposal} taskId={taskId} />
    case 'spec_task':
      return <SpecTaskProposalCard proposal={proposal} taskId={taskId} />
    case 'mark_complete':
      return <MarkCompleteProposalCard proposal={proposal} taskId={taskId} />
    default:
      return null
  }
}

function PRProposalCard({ proposal, taskId }: { proposal: TypesSpecTaskProposal; taskId: string }) {
  const { decideAsync, isDeciding } = useSpecTaskProposals(taskId)
  const snackbar = useSnackbar()
  const [headBranch, setHeadBranch] = useState(proposal.pr_head_branch || '')
  const [baseBranch, setBaseBranch] = useState(proposal.pr_base_branch || '')
  const [title, setTitle] = useState(proposal.pr_title || '')
  const [body, setBody] = useState(proposal.pr_body || '')
  const [comment, setComment] = useState('')

  const handleApprove = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: {
          decision: 'approve',
          comment,
          edited_payload: stringifyEdits({
            pr_head_branch: headBranch,
            pr_base_branch: baseBranch,
            pr_title: title,
            pr_body: body,
          }),
        },
      })
      snackbar.success('Pull request opening — check back in a moment')
    } catch (err) {
      snackbar.error(`Failed to approve: ${err}`)
    }
  }

  const handleReject = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: { decision: 'reject', comment },
      })
      snackbar.info('Proposal rejected')
    } catch (err) {
      snackbar.error(`Failed to reject: ${err}`)
    }
  }

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
          <Chip label="Pull Request proposal" color="primary" size="small" />
          <Typography variant="caption" color="text.secondary">
            {new Date(proposal.created_at || '').toLocaleString()}
          </Typography>
        </Stack>

        {proposal.agent_reason && (
          <Alert severity="info" sx={{ mb: 2 }}>
            <Typography variant="body2"><strong>Agent reason:</strong> {proposal.agent_reason}</Typography>
          </Alert>
        )}

        <Stack spacing={1.5}>
          <Stack direction="row" spacing={1.5}>
            <TextField
              label="Head branch"
              value={headBranch}
              onChange={(e) => setHeadBranch(e.target.value)}
              size="small"
              fullWidth
              helperText="Defaults to the task's branch when empty"
            />
            <TextField
              label="Base branch"
              value={baseBranch}
              onChange={(e) => setBaseBranch(e.target.value)}
              size="small"
              fullWidth
              helperText="Defaults to repo's default branch"
            />
          </Stack>
          <TextField
            label="PR title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            size="small"
          />
          <TextField
            label="PR body"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            size="small"
            multiline
            minRows={3}
            maxRows={10}
          />
          <TextField
            label="Optional reviewer note (sent to agent)"
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            size="small"
          />
        </Stack>

        <Divider sx={{ my: 2 }} />
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button onClick={handleReject} disabled={isDeciding} color="error">Reject</Button>
          <Button onClick={handleApprove} disabled={isDeciding} variant="contained" color="success">
            {isDeciding ? 'Working…' : 'Approve & Open PR'}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  )
}

function SpecTaskProposalCard({ proposal, taskId }: { proposal: TypesSpecTaskProposal; taskId: string }) {
  const { decideAsync, isDeciding } = useSpecTaskProposals(taskId)
  const snackbar = useSnackbar()
  const [name, setName] = useState(proposal.task_name || '')
  const [description, setDescription] = useState(proposal.task_description || '')
  const [type, setType] = useState(proposal.task_type || 'feature')
  const [priority, setPriority] = useState(proposal.task_priority || 'medium')
  const [comment, setComment] = useState('')

  const handleApprove = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: {
          decision: 'approve',
          comment,
          edited_payload: stringifyEdits({
            task_name: name,
            task_description: description,
            task_type: type,
            task_priority: priority,
          }),
        },
      })
      snackbar.success('New spec task created in the project backlog')
    } catch (err) {
      snackbar.error(`Failed to approve: ${err}`)
    }
  }

  const handleReject = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: { decision: 'reject', comment },
      })
      snackbar.info('Proposal rejected')
    } catch (err) {
      snackbar.error(`Failed to reject: ${err}`)
    }
  }

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
          <Chip label="Spec task proposal" color="secondary" size="small" />
          <Typography variant="caption" color="text.secondary">
            {new Date(proposal.created_at || '').toLocaleString()}
          </Typography>
        </Stack>

        {proposal.agent_reason && (
          <Alert severity="info" sx={{ mb: 2 }}>
            <Typography variant="body2"><strong>Agent reason:</strong> {proposal.agent_reason}</Typography>
          </Alert>
        )}

        <Stack spacing={1.5}>
          <TextField label="Task name" value={name} onChange={(e) => setName(e.target.value)} size="small" />
          <TextField
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            size="small"
            multiline
            minRows={3}
          />
          <Stack direction="row" spacing={1.5}>
            <TextField select label="Type" value={type} onChange={(e) => setType(e.target.value)} size="small" fullWidth>
              <MenuItem value="feature">feature</MenuItem>
              <MenuItem value="bug">bug</MenuItem>
              <MenuItem value="refactor">refactor</MenuItem>
            </TextField>
            <TextField select label="Priority" value={priority} onChange={(e) => setPriority(e.target.value)} size="small" fullWidth>
              <MenuItem value="low">low</MenuItem>
              <MenuItem value="medium">medium</MenuItem>
              <MenuItem value="high">high</MenuItem>
              <MenuItem value="critical">critical</MenuItem>
            </TextField>
          </Stack>
          <TextField
            label="Optional reviewer note (sent to agent)"
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            size="small"
          />
        </Stack>

        <Divider sx={{ my: 2 }} />
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button onClick={handleReject} disabled={isDeciding} color="error">Reject</Button>
          <Button onClick={handleApprove} disabled={isDeciding} variant="contained" color="success">
            {isDeciding ? 'Working…' : 'Approve & Create Task'}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  )
}

function MarkCompleteProposalCard({ proposal, taskId }: { proposal: TypesSpecTaskProposal; taskId: string }) {
  const { decideAsync, isDeciding } = useSpecTaskProposals(taskId)
  const snackbar = useSnackbar()
  const [feedback, setFeedback] = useState('')

  const handleConfirm = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: { decision: 'approve' },
      })
      snackbar.success('Task marked done')
    } catch (err) {
      snackbar.error(`Failed to confirm: ${err}`)
    }
  }

  const handleSendBack = async () => {
    try {
      await decideAsync({
        proposalId: proposal.id!,
        request: { decision: 'reject', comment: feedback },
      })
      snackbar.info('Sent back to agent with feedback')
    } catch (err) {
      snackbar.error(`Failed to send back: ${err}`)
    }
  }

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
          <Chip label="Mark task complete" color="success" size="small" />
          <Typography variant="caption" color="text.secondary">
            {new Date(proposal.created_at || '').toLocaleString()}
          </Typography>
        </Stack>

        {proposal.complete_reason && (
          <Alert severity="success" sx={{ mb: 2 }}>
            <Typography variant="body2"><strong>Agent's summary:</strong> {proposal.complete_reason}</Typography>
          </Alert>
        )}

        <TextField
          label="Feedback (only used if you Send Back)"
          value={feedback}
          onChange={(e) => setFeedback(e.target.value)}
          size="small"
          multiline
          minRows={2}
          fullWidth
        />

        <Divider sx={{ my: 2 }} />
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button onClick={handleSendBack} disabled={isDeciding} color="warning">Send Back</Button>
          <Button onClick={handleConfirm} disabled={isDeciding} variant="contained" color="success">
            {isDeciding ? 'Working…' : 'Mark Done'}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  )
}

// stringifyEdits returns the edited_payload object suitable for the API,
// or undefined when there are no edits. (Backend accepts a JSON object.)
function stringifyEdits(edits: Record<string, string | undefined>): Record<string, any> | undefined {
  const cleaned: Record<string, string> = {}
  for (const [k, v] of Object.entries(edits)) {
    if (v !== undefined && v !== '') cleaned[k] = v
  }
  if (Object.keys(cleaned).length === 0) return undefined
  return cleaned
}
