import React, { FC, useState } from 'react'
import {
  Button,
  ButtonGroup,
  Menu,
  MenuItem,
  Typography,
  Box,
  Tooltip,
} from '@mui/material'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import { FilePlus, Package } from 'lucide-react'

import { TypesSampleProject } from '../../api/api'
import { getSampleProjectIcon } from '../../utils/sampleProjectIcons'

// Component for creating new projects with sample templates
interface CreateProjectButtonProps {
  onCreateEmpty: () => void
  onCreateFromSample: (sampleId: string, sampleName: string) => void
  sampleProjects: TypesSampleProject[]
  isCreating?: boolean
  variant?: 'contained' | 'outlined' | 'text'
  color?: 'primary' | 'secondary'
  size?: 'small' | 'medium' | 'large'
  showDropdownArrow?: boolean
}

const CreateProjectButton: FC<CreateProjectButtonProps> = ({
  onCreateEmpty,
  onCreateFromSample,
  sampleProjects,
  isCreating = false,
  variant = 'contained',
  color = 'secondary',
  size = 'medium',
  showDropdownArrow = true,
}) => {
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null)

  const handleMenuClose = () => {
    setMenuAnchor(null)
  }

  const handleEmptyProject = () => {
    handleMenuClose()
    onCreateEmpty()
  }

  const handleSampleProject = (sampleId: string, sampleName: string) => {
    handleMenuClose()
    onCreateFromSample(sampleId, sampleName)
  }

  if (!showDropdownArrow) {
    // Simple button without dropdown
    return (
      <Button
        variant={variant}
        color={color}
        size={size}
        startIcon={<FilePlus size={20} />}
        onClick={onCreateEmpty}
        disabled={isCreating}
      >
        {variant === 'text' ? 'Create Project' : 'New Project'}
      </Button>
    )
  }

  return (
    <>
      <ButtonGroup variant={variant} color={color} size={size}>
        <Button
          startIcon={<FilePlus size={20} />}
          onClick={handleEmptyProject}
          disabled={isCreating}
        >
          {variant === 'text' ? 'Create Project' : 'New Project'}
        </Button>
        <Tooltip
          title={
            <Box>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Start from a template
              </Typography>
              <Typography variant="caption">
                {sampleProjects.length} sample project{sampleProjects.length !== 1 ? 's' : ''} available
              </Typography>
            </Box>
          }
          arrow
        >
          <Button
            onClick={(e) => setMenuAnchor(e.currentTarget)}
            disabled={isCreating}
            sx={{
              px: 1,
              minWidth: 'auto',
            }}
          >
            <Package size={18} />
            <ArrowDropDownIcon sx={{ ml: 0.25, fontSize: 18 }} />
          </Button>
        </Tooltip>
      </ButtonGroup>

      <Menu
        anchorEl={menuAnchor}
        open={Boolean(menuAnchor)}
        onClose={handleMenuClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
      >
        {sampleProjects.length === 0 ? (
          <MenuItem disabled>
            <Typography variant="body2" color="text.secondary">
              No sample projects available
            </Typography>
          </MenuItem>
        ) : (
          sampleProjects.map((sample) => (
          <Tooltip
            key={`tooltip-${sample.id}`}
            title={
              <Box>
                <Typography variant="body2" sx={{ mb: 0.5 }}>
                  {sample.description || 'Sample project with pre-configured tasks'}
                </Typography>
                <Typography variant="caption" sx={{ opacity: 0.8 }}>
                  {sample.category} • {sample.difficulty}
                </Typography>
              </Box>
            }
            placement="right"
            arrow
          >
            <span>
              <MenuItem
                onClick={() => handleSampleProject(sample.id || '', sample.name)}
                disabled={isCreating}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 200 }}>
                  {getSampleProjectIcon(sample.id, sample.category)}
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    {sample.name}
                  </Typography>
                </Box>
              </MenuItem>
            </span>
          </Tooltip>
        )))}
      </Menu>
    </>
  )
}

export default CreateProjectButton
