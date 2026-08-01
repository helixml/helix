import { FC, useState } from 'react'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Divider from '@mui/material/Divider'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import FilterListIcon from '@mui/icons-material/FilterList'

import { ChartTopicFilter, CHART_TOPIC_FILTERS } from './chartTopicVisibility'

export const chartToolbarButtonSizeSx = {
  minWidth: 0,
  height: 30,
  px: 1,
  fontSize: '0.72rem',
  '& .MuiButton-startIcon': { mr: 0.5 },
}

export const chartToolbarButtonSx = {
  ...chartToolbarButtonSizeSx,
  color: 'text.secondary',
  borderColor: 'divider',
  backgroundColor: 'background.paper',
  '&:hover': {
    color: 'text.primary',
    borderColor: 'text.disabled',
    backgroundColor: 'action.hover',
  },
}

const ChartTopicVisibilityMenu: FC<{
  selected: ChartTopicFilter[]
  onChange: (selected: ChartTopicFilter[]) => void
}> = ({ selected, onChange }) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const allSelected = selected.length === CHART_TOPIC_FILTERS.length
  const someSelected = selected.length > 0 && !allSelected

  const toggleFilter = (filter: ChartTopicFilter) => {
    if (selected.includes(filter)) {
      onChange(selected.filter((candidate) => candidate !== filter))
      return
    }
    onChange(CHART_TOPIC_FILTERS
      .map((candidate) => candidate.id)
      .filter((candidate) => candidate === filter || selected.includes(candidate)))
  }

  return (
    <>
      <Button
        size="small"
        variant="outlined"
        startIcon={<FilterListIcon />}
        onClick={(event) => setAnchorEl(event.currentTarget)}
        aria-label="Choose topic types shown on chart"
        aria-haspopup="menu"
        aria-expanded={Boolean(anchorEl)}
        sx={chartToolbarButtonSx}
      >
        Topics {selected.length}/{CHART_TOPIC_FILTERS.length}
      </Button>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        MenuListProps={{ dense: true, sx: { py: 0.5 } }}
        PaperProps={{ sx: { minWidth: 200 } }}
      >
        <MenuItem
          dense
          onClick={() => onChange(allSelected ? [] : CHART_TOPIC_FILTERS.map((filter) => filter.id))}
          sx={{ minHeight: 32, px: 1 }}
        >
          <Checkbox
            size="small"
            checked={allSelected}
            indeterminate={someSelected}
            sx={{
              p: 0.5,
              mr: 0.5,
              color: 'text.disabled',
              '&.Mui-checked, &.MuiCheckbox-indeterminate': { color: 'text.secondary' },
            }}
          />
          <ListItemText primary="All topic types" primaryTypographyProps={{ fontSize: '0.8rem', fontWeight: 600 }} />
        </MenuItem>
        <Divider sx={{ my: 0.5 }} />
        {CHART_TOPIC_FILTERS.map((filter) => (
          <MenuItem key={filter.id} dense onClick={() => toggleFilter(filter.id)} sx={{ minHeight: 32, px: 1 }}>
            <Checkbox
              size="small"
              checked={selected.includes(filter.id)}
              sx={{
                p: 0.5,
                mr: 0.5,
                color: 'text.disabled',
                '&.Mui-checked': { color: 'text.secondary' },
              }}
            />
            <ListItemText primary={filter.label} primaryTypographyProps={{ fontSize: '0.8rem' }} />
          </MenuItem>
        ))}
      </Menu>
    </>
  )
}

export default ChartTopicVisibilityMenu
