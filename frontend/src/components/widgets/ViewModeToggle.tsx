import { FC } from 'react'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import { LayoutGrid, Rows3 } from 'lucide-react'

export type ViewMode = 'table' | 'cards'

interface ViewModeToggleProps {
  mode: ViewMode
  onChange: (mode: ViewMode) => void
  tableLabel?: string
  cardsLabel?: string
}

const ViewModeToggle: FC<ViewModeToggleProps> = ({
  mode,
  onChange,
  tableLabel = 'Table view',
  cardsLabel = 'Card view',
}) => {
  const buttonSx = (active: boolean) => ({
    width: 32,
    height: 32,
    borderRadius: 1,
    bgcolor: active ? 'background.paper' : 'transparent',
    boxShadow: active ? 1 : 0,
    color: active ? 'primary.main' : 'text.secondary',
    '&:hover': {
      bgcolor: active ? 'background.paper' : 'action.selected',
      color: active ? 'primary.main' : 'text.primary',
    },
  })

  return (
    <Stack
      direction="row"
      spacing={0.5}
      sx={{
        borderRadius: 1.5,
        p: 0.5,
        bgcolor: 'rgba(255,255,255,0.06)',
      }}
    >
      <Tooltip title={tableLabel}>
        <IconButton size="small" onClick={() => onChange('table')} sx={buttonSx(mode === 'table')}>
          <Rows3 size={16} />
        </IconButton>
      </Tooltip>
      <Tooltip title={cardsLabel}>
        <IconButton size="small" onClick={() => onChange('cards')} sx={buttonSx(mode === 'cards')}>
          <LayoutGrid size={16} />
        </IconButton>
      </Tooltip>
    </Stack>
  )
}

export default ViewModeToggle
