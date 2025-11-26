import React, { FC } from 'react'
import {
  Box,
  FormControl,
  Select,
  MenuItem,
  Button,
} from '@mui/material'
import { GitBranch, Plus } from 'lucide-react'

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
          renderValue={(value) => (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <GitBranch size={14} />
              <span>{value || fallbackBranch}</span>
            </Box>
          )}
          sx={{ fontWeight: 500 }}
        >
          {branches?.map((branch) => (
            <MenuItem key={branch} value={branch}>
              {branch}
            </MenuItem>
          ))}
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

