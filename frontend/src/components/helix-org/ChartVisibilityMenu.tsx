import { FC, ReactElement, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Divider from '@mui/material/Divider'
import InputAdornment from '@mui/material/InputAdornment'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import TextField from '@mui/material/TextField'
import SearchIcon from '@mui/icons-material/Search'

import { matchesAllTokens } from '../../utils/searchUtils'

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

export type ChartVisibilityOption = {
  id: string
  label: string
}

const checkboxSx = {
  p: 0.5,
  mr: 0.5,
  color: 'text.disabled',
  '&.Mui-checked, &.MuiCheckbox-indeterminate': { color: 'text.secondary' },
}

const ChartVisibilityMenu: FC<{
  label: string
  icon: ReactElement
  options: readonly ChartVisibilityOption[]
  selected: string[]
  onChange: (selected: string[]) => void
  allLabel?: string
  // counts overrides the button's "shown/total" badge for menus whose
  // options are categories rather than the entities themselves — the
  // operator cares how many things are on the chart, not how many
  // checkboxes are ticked.
  counts?: { shown: number; total: number }
}> = ({ label, icon, options, selected, onChange, allLabel = `All ${label.toLowerCase()}`, counts }) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [query, setQuery] = useState('')
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const filteredOptions = useMemo(
    () => options.filter((option) => matchesAllTokens(query, option.label, option.id)),
    [options, query],
  )
  const selectedResultCount = filteredOptions.filter((option) => selectedSet.has(option.id)).length
  const allResultsSelected = filteredOptions.length > 0 && selectedResultCount === filteredOptions.length
  const someResultsSelected = selectedResultCount > 0 && !allResultsSelected

  const close = () => {
    setAnchorEl(null)
    setQuery('')
  }

  const toggleOption = (id: string) => {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onChange(options.map((option) => option.id).filter((optionID) => next.has(optionID)))
  }

  const toggleFilteredOptions = () => {
    const next = new Set(selected)
    filteredOptions.forEach((option) => {
      if (allResultsSelected) next.delete(option.id)
      else next.add(option.id)
    })
    onChange(options.map((option) => option.id).filter((optionID) => next.has(optionID)))
  }

  return (
    <>
      <Button
        size="small"
        variant="outlined"
        startIcon={icon}
        onClick={(event) => setAnchorEl(event.currentTarget)}
        aria-label={`Choose ${label.toLowerCase()} shown on chart`}
        aria-haspopup="menu"
        aria-expanded={Boolean(anchorEl)}
        sx={chartToolbarButtonSx}
      >
        {label} {counts ? counts.shown : selected.length}/{counts ? counts.total : options.length}
      </Button>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={close}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        MenuListProps={{ dense: true, sx: { py: 0.5 } }}
        PaperProps={{ sx: { width: 240, maxHeight: 380 } }}
      >
        <Box sx={{ px: 1, pt: 0.5, pb: 0.75 }}>
          <TextField
            autoFocus
            fullWidth
            size="small"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => event.stopPropagation()}
            placeholder={`Search ${label.toLowerCase()}`}
            inputProps={{ 'aria-label': `Search ${label.toLowerCase()}` }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 16 }} />
                </InputAdornment>
              ),
              sx: { height: 32, fontSize: '0.8rem' },
            }}
          />
        </Box>
        <MenuItem
          dense
          disabled={filteredOptions.length === 0}
          onClick={toggleFilteredOptions}
          sx={{ minHeight: 32, px: 1 }}
        >
          <Checkbox
            size="small"
            checked={allResultsSelected}
            indeterminate={someResultsSelected}
            sx={checkboxSx}
          />
          <ListItemText
            primary={query.trim() ? 'All search results' : allLabel}
            primaryTypographyProps={{ fontSize: '0.8rem', fontWeight: 600 }}
          />
        </MenuItem>
        <Divider sx={{ my: 0.5 }} />
        {filteredOptions.map((option) => (
          <MenuItem key={option.id} dense onClick={() => toggleOption(option.id)} sx={{ minHeight: 32, px: 1 }}>
            <Checkbox size="small" checked={selectedSet.has(option.id)} sx={checkboxSx} />
            <ListItemText
              primary={option.label || option.id}
              primaryTypographyProps={{ fontSize: '0.8rem', noWrap: true }}
            />
          </MenuItem>
        ))}
        {filteredOptions.length === 0 && (
          <Box sx={{ px: 1.5, py: 1, color: 'text.secondary', fontSize: '0.78rem' }}>
            No matches
          </Box>
        )}
      </Menu>
    </>
  )
}

export default ChartVisibilityMenu
