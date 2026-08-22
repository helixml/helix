import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import { Pencil } from 'lucide-react'

import ToolPickerDialog from '../helix-org/ToolPickerDialog'
import { useAgentToolCatalogue } from '../../services/agentToolsService'
import { NEUTRAL_ACTION_BUTTON_SX } from '../../styles/actionButtons'

interface AgentToolsPickerProps {
  /** Tools owned by this scope — the ones a save writes back. */
  selectedTools: string[]
  onChange: (tools: string[]) => void
  /** Tools inherited from the project; shown enabled but read-only. */
  lockedTools?: string[]
  disabled?: boolean
  helperText?: string
  /** Rendered inline on the same row as the count and Edit button. */
  label?: string
}

// AgentToolsPicker summarises the current Helix MCP tool grant as a count with
// an Edit affordance; the dialog carries the detail. Used at both scopes: a
// project sets the floor for its tasks, and a task adds extras on top (its
// project's tools arrive as lockedTools).
const AgentToolsPicker: FC<AgentToolsPickerProps> = ({
  selectedTools,
  onChange,
  lockedTools,
  disabled = false,
  helperText,
  label,
}) => {
  const [editing, setEditing] = useState(false)
  const { data: catalogue = [] } = useAgentToolCatalogue()

  const locked = lockedTools ?? []
  const effective = useMemo(
    () => Array.from(new Set([...locked, ...selectedTools])).sort(),
    [locked, selectedTools],
  )

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {label && (
          <Typography variant="subtitle2" color="text.secondary" sx={{ flexShrink: 0 }}>
            {label}
          </Typography>
        )}
        <Typography variant="body2" color="text.secondary">
          {effective.length === 0
            ? 'No tools enabled'
            : `${effective.length} tool${effective.length === 1 ? '' : 's'} enabled`}
        </Typography>
        <Button
          size="small"
          variant="text"
          startIcon={<Pencil size={16} />}
          onClick={() => setEditing(true)}
          disabled={disabled}
          sx={{ ...NEUTRAL_ACTION_BUTTON_SX, ml: 'auto', flexShrink: 0 }}
        >
          Edit
        </Button>
      </Box>

      <ToolPickerDialog
        open={editing}
        tools={catalogue.map((tool) => ({ name: tool.name, description: tool.description }))}
        selectedTools={selectedTools}
        lockedTools={locked}
        helperText={helperText}
        onClose={() => setEditing(false)}
        onApply={onChange}
      />
    </Box>
  )
}

export default AgentToolsPicker
