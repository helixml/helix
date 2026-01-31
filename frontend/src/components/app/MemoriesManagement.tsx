import React, { FC, useMemo, useCallback, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'
import Stack from '@mui/material/Stack'
import { Trash } from 'lucide-react'

import SimpleTable from '../widgets/SimpleTable'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useTheme from '@mui/material/styles/useTheme'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'

import { TypesMemory } from '../../api/api'
import { useListAppMemories, useDeleteAppMemory } from '../../services/appService'

interface MemoriesManagementProps {
  appId: string
  memory: boolean
  onMemoryChange: (value: boolean) => void
  readOnly?: boolean
}

const MemoriesManagement: FC<MemoriesManagementProps> = ({ appId, memory, onMemoryChange, readOnly = false }) => {
  const theme = useTheme()
  const snackbar = useSnackbar()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentMemory, setCurrentMemory] = useState<TypesMemory | null>(null)
  const [deletingMemory, setDeletingMemory] = useState<string | null>(null)

  const { data: memoriesResponse, isLoading } = useListAppMemories(appId)
  const memories = memoriesResponse?.data || []
  const deleteMemoryMutation = useDeleteAppMemory(appId)

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, memory: TypesMemory) => {
    setAnchorEl(event.currentTarget)
    setCurrentMemory(memory)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentMemory(null)
  }

  const handleDelete = () => {
    if (currentMemory?.id) {
      setDeletingMemory(currentMemory.id)
    }
    handleMenuClose()
  }

  const handleConfirmDelete = async () => {
    if (deletingMemory) {
      try {
        await deleteMemoryMutation.mutateAsync(deletingMemory)
        snackbar.success('Memory deleted successfully')
      } catch (error) {
        snackbar.error('Failed to delete memory')
      }
    }
    setDeletingMemory(null)
  }

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Unknown'
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const truncateContent = (content?: string, maxLength: number = 100) => {
    if (!content) return 'No content'
    if (content.length <= maxLength) return content
    return content.substring(0, maxLength) + '...'
  }

  const tableData = useMemo(() => {
    return memories.map(memory => ({
      id: memory.id,
      _data: memory,
      created_at: (
        <Typography variant="body2" color="text.secondary">
          {formatDate(memory.created)}
        </Typography>
      ),
      contents: (
        <Box sx={{ maxWidth: 400 }}>
          <Typography 
            variant="body2" 
            sx={{ 
              wordBreak: 'break-word',
              whiteSpace: 'pre-wrap'
            }}
          >
            {truncateContent(memory.contents)}
          </Typography>
        </Box>
      ),
    }))
  }, [memories])

  const getActions = useCallback((memory: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="memory-menu"
        aria-haspopup="true"
        onClick={(e) => handleMenuClick(e, memory._data)}
      >
        <MoreVertIcon />
      </IconButton>
    )
  }, [])

  const tableFields = [
    {
      name: 'created_at',
      title: 'Created At',
    },
    {
      name: 'contents',
      title: 'Contents',
    }
  ]

  if (isLoading) {
    return (
      <Box sx={{ p: 3 }}>
        <Typography variant="body2" color="text.secondary">
          Loading memories...
        </Typography>
      </Box>
    )
  }

  return (
    <>
      <Box sx={{ p: 3 }}>
        <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between" sx={{ mb: 2 }}>
          <Typography variant="h6">
            Agent Memories
          </Typography>
          <FormControlLabel
            control={
              <Switch
                checked={memory}
                onChange={(e) => onMemoryChange(e.target.checked)}
                disabled={readOnly}
              />
            }
            label="Memory"
          />
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Memories are automatically injected into the agent's context and can be used to remember things between sessions.          
          They are not shared between users or agents, only between sessions for the same user.
        </Typography>
        {memories.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No memories found for this agent.
          </Typography>
        ) : (
          <SimpleTable
            authenticated={true}
            fields={tableFields}
            data={tableData}
            getActions={getActions}
          />
        )}
      </Box>
      
      <Menu
        id="memory-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleDelete} disabled={deleteMemoryMutation.isPending}>
          <Trash size={16} style={{ marginRight: 5 }} />
          Delete
        </MenuItem>
      </Menu>
      
      {deletingMemory && (
        <DeleteConfirmWindow
          title="this memory"
          onSubmit={handleConfirmDelete}
          onCancel={() => setDeletingMemory(null)}
        />
      )}
    </>
  )
}

export default MemoriesManagement
