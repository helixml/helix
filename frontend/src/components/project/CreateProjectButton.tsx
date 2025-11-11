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
import {
  Kanban,
  FilePlus,
  CheckSquare,
  Layers,
  Code,
  Zap,
  Server,
  Palette,
  Triangle,
  Coffee,
  Briefcase,
  Mail,
  Share2,
  DollarSign,
  Target,
  Users,
  FileEdit,
  BookOpen,
  Megaphone,
  PenTool,
  MessageSquare,
  Newspaper,
  Package,
} from 'lucide-react'

import { TypesSampleProject } from '../../api/api'

// Map sample project IDs to Lucide icons
const getSampleProjectIcon = (sampleId?: string, category?: string) => {
  const iconSize = 18

  // Map by specific ID first
  const iconMap: Record<string, JSX.Element> = {
    // Empty/Generic
    'empty': <FilePlus size={iconSize} />,
    'blank': <FilePlus size={iconSize} />,

    // Development Projects
    'nodejs-todo': <CheckSquare size={iconSize} />,
    'react-app': <Layers size={iconSize} />,
    'python-api': <Code size={iconSize} />,
    'nextjs-app': <Triangle size={iconSize} />,
    'express-api': <Zap size={iconSize} />,
    'vue-app': <Palette size={iconSize} />,
    'angular-app': <Triangle size={iconSize} />,
    'django-app': <Server size={iconSize} />,
    'flask-api': <Server size={iconSize} />,
    'spring-boot': <Coffee size={iconSize} />,

    // Business Tasks
    'linkedin-outreach': <Briefcase size={iconSize} />,
    'email-campaign': <Mail size={iconSize} />,
    'social-media': <Share2 size={iconSize} />,
    'sales-automation': <DollarSign size={iconSize} />,
    'lead-generation': <Target size={iconSize} />,
    'customer-service': <Users size={iconSize} />,

    // Content Creation
    'blog-posts': <FileEdit size={iconSize} />,
    'helix-blog-posts': <FileEdit size={iconSize} />,
    'documentation': <BookOpen size={iconSize} />,
    'marketing-content': <Megaphone size={iconSize} />,
    'technical-writing': <PenTool size={iconSize} />,
    'social-posts': <MessageSquare size={iconSize} />,
    'newsletter': <Newspaper size={iconSize} />,
  }

  if (sampleId && iconMap[sampleId]) {
    return iconMap[sampleId]
  }

  // Fallback to category-based icons
  if (category) {
    switch (category.toLowerCase()) {
      case 'development':
        return <Code size={iconSize} />
      case 'business':
        return <Briefcase size={iconSize} />
      case 'content':
        return <FileEdit size={iconSize} />
      default:
        return <Package size={iconSize} />
    }
  }

  // Default fallback
  return <Kanban size={iconSize} />
}

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
        <Button
          size="small"
          onClick={(e) => setMenuAnchor(e.currentTarget)}
          sx={{ px: 1 }}
          disabled={isCreating}
        >
          <ArrowDropDownIcon />
        </Button>
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
        <Tooltip
          title="Create a blank project with no sample code or pre-configured tasks"
          placement="right"
          arrow
        >
          <MenuItem onClick={handleEmptyProject}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 200 }}>
              <FilePlus size={18} />
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Empty Project
              </Typography>
            </Box>
          </MenuItem>
        </Tooltip>

        {sampleProjects.length > 0 && (
          <MenuItem disabled>
            <Typography variant="caption" sx={{ fontWeight: 600, opacity: 0.6 }}>
              Sample Projects
            </Typography>
          </MenuItem>
        )}

        {sampleProjects.map((sample) => (
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
        ))}
      </Menu>
    </>
  )
}

export default CreateProjectButton
