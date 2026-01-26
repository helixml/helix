import React, { FC } from 'react'
import {
  Box,
  FormControl,
  Select,
  MenuItem,
  Button,
  Tooltip,
  Typography,
} from '@mui/material'
import { GitBranch, Plus, ArrowDown, ArrowUp } from 'lucide-react'

interface BranchSelectProps {
  repository: any
  currentBranch: string
  setCurrentBranch: (branch: string) => void
  branches: string[]
  showNewBranchButton?: boolean
  onBranchChange?: (branch: string) => void
  onNewBranchClick?: () => void
  size?: 'small' | 'medium'
}

const getFallbackBranch = (defaultBranch: string | undefined, branches: string[] | null | undefined): string => {
  if (!branches || branches.length === 0) {
    return ''
  }

  if (branches.includes('main')) {
    return 'main'
  }
  if (branches.includes('master')) {
    return 'master'
  }

  if (defaultBranch && branches.includes(defaultBranch)) {
    return defaultBranch
  }

  return branches[0] || ''
}

const BranchSelect: FC<BranchSelectProps> = ({
  repository,
  currentBranch,
  setCurrentBranch,
  branches,
  showNewBranchButton = false,
  onBranchChange,
  onNewBranchClick,
  size = 'small',
}) => {
  const fallbackBranch = getFallbackBranch(repository?.default_branch, branches)

  const handleChange = (value: string) => {
    setCurrentBranch(value)
    if (onBranchChange) {
      onBranchChange(value)
    }
  }

  return (
    <>
      <FormControl size={size} sx={{ minWidth: 200 }}>
        <Select
          value={currentBranch}
          onChange={(e) => {
            handleChange(e.target.value)
          }}
          displayEmpty
          renderValue={(value) => {
            const branchName = value || fallbackBranch
            const isDefaultBranch = branchName === repository?.default_branch || branchName === fallbackBranch
            return (
              <Tooltip title={isDefaultBranch ? "Default branch - pulls from upstream" : "Feature branch - pushes to upstream"}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <GitBranch size={14} />
                  <span>{branchName}</span>
                  {isDefaultBranch ? (
                    <Box component="span" sx={{ display: 'flex', alignItems: 'center', color: 'info.main', fontSize: '0.75rem' }}>
                      <ArrowDown size={12} />
                    </Box>
                  ) : (
                    <Box component="span" sx={{ display: 'flex', alignItems: 'center', color: 'success.main', fontSize: '0.75rem' }}>
                      <ArrowUp size={12} />
                    </Box>
                  )}
                </Box>
              </Tooltip>
            )
          }}
          sx={{ fontWeight: 500 }}
        >
          {branches?.map((branch) => {
            const isDefault = branch === repository?.default_branch || branch === fallbackBranch
            return (
              <MenuItem key={branch} value={branch}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                  <span>{branch}</span>
                  {isDefault ? (
                    <Tooltip title="Pulls from upstream">
                      <Box component="span" sx={{ display: 'flex', alignItems: 'center', color: 'info.main', ml: 'auto' }}>
                        <ArrowDown size={14} />
                        <Typography variant="caption" sx={{ ml: 0.5 }}>PULL</Typography>
                      </Box>
                    </Tooltip>
                  ) : (
                    <Tooltip title="Pushes to upstream">
                      <Box component="span" sx={{ display: 'flex', alignItems: 'center', color: 'success.main', ml: 'auto' }}>
                        <ArrowUp size={14} />
                        <Typography variant="caption" sx={{ ml: 0.5 }}>PUSH</Typography>
                      </Box>
                    </Tooltip>
                  )}
                </Box>
              </MenuItem>
            )
          })}
        </Select>
      </FormControl>
      {showNewBranchButton && onNewBranchClick && (
        <Button
          startIcon={<Plus size={16} />}
          variant="outlined"
          size={size}
          onClick={onNewBranchClick}
          sx={{ height: size === 'small' ? 40 : undefined, whiteSpace: 'nowrap' }}
        >
          New Branch
        </Button>
      )}
    </>
  )
}

export default BranchSelect

