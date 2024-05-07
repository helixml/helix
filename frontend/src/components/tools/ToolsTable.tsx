import React, { FC, useMemo, useCallback } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import ViewIcon from '@mui/icons-material/Visibility'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'
import useAccount from '../../hooks/useAccount'

import ToolDetail from './ToolDetail'

import useTheme from '@mui/material/styles/useTheme'

import {
  ITool,
} from '../../types'

const ToolsDataGrid: FC<React.PropsWithChildren<{
  data: ITool[],
  onEdit: (tool: ITool) => void,
  onDelete: (tool: ITool) => void,
}>> = ({
  data,
  onEdit,
  onDelete,
}) => {

  const theme = useTheme()
  const account = useAccount()

  const isAdmin = account.admin

  const tableData = useMemo(() => {
    return data.map(tool => {
      const accessType = tool.global ? 'Global' : 'User'
      const toolType = tool.config.gptscript ? 'GPT Script' : 'Tool'
    
      return {
        id: tool.id,
        _data: tool,
        name: (
          <a
            style={{
              textDecoration: 'none',
              fontWeight: 'bold',
              color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            }}
            href="#"
            onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
              e.preventDefault()
              e.stopPropagation()
              onEdit(tool)
            }}
          >
            { tool.name }
          </a>
        ),
        type: `${toolType} (${accessType})`,
        updated: (
          <Box
            sx={{
              fontSize: '0.9em',
            }}
          >
            { new Date(tool.updated).toLocaleString() }
          </Box>
        ),
        details: (
          <ToolDetail
            tool={ tool }
          />
        ),
      }
    })
  }, [
    theme,
    data,
  ])

  const getActions = useCallback((tool: any) => {
    if(tool.global && !isAdmin) {
      return (
        <Box
          sx={{
            width: '100%',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'flex-end',
            justifyContent: 'space-between',
            pl: 2,
            pr: 2,
          }}
        >
          <ClickLink
            onClick={ () => {
              onEdit(tool._data)
            }}
          >
            <ViewIcon />
          </ClickLink>
        </Box>
      )
    } else {
      return (
        <Box
          sx={{
            width: '100%',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'flex-end',
            justifyContent: 'flex-end',
            pl: 2,
            pr: 2,
          }}
        >
          <ClickLink
            sx={{mr:2}}
            onClick={ () => {
              onDelete(tool._data)
            }}
          >
            <Tooltip title="Delete">
              <DeleteIcon />
            </Tooltip>
          </ClickLink>
        
          <ClickLink
            onClick={ () => {
              onEdit(tool._data)
            }}
          >
            <Tooltip title="Edit">
              <EditIcon />
            </Tooltip>
          </ClickLink>
        </Box>
      )
    }
  }, [

  ])

  return (
    <SimpleTable
      fields={[{
        name: 'name',
        title: 'Name',
      }, {
        name: 'type',
        title: 'Type'
      }, {
        name: 'updated',
        title: 'Updated',
      }, {
        name: 'details',
        title: 'Details',
      }]}
      data={ tableData }
      getActions={ getActions }
    />
  )
}

export default ToolsDataGrid