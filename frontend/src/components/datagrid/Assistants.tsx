import React, { FC, useMemo } from 'react'
import VisibilityIcon from '@mui/icons-material/Visibility'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import FolderIcon from '@mui/icons-material/Folder'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import { prettyBytes } from '../../utils/format'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import ClickLink from '../widgets/ClickLink'
import useAccount from '../../hooks/useAccount'

import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  IAssistant,
} from '../../types'

const ToolsDataGrid: FC<React.PropsWithChildren<{
  data: IAssistant[],
  onEdit: (tool: IAssistant) => void,
  onDelete: (tool: IAssistant) => void,
}>> = ({
  data,
  onEdit,
  onDelete,
}) => {

  const theme = useTheme()
  const account = useAccount()

  const columns = useMemo<IDataGrid2_Column<IAssistant>[]>(() => {
    return [
    {
      name: 'name',
      header: 'Name',
      defaultFlex: 1,
      render: ({ data }) => {
        return (
          <a
            style={{
              textDecoration: 'none',
              color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            }}
            href="#"
            onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
              e.preventDefault()
              e.stopPropagation()
              onEdit(data)
            }}
          >
            { data.name }
          </a>
        )
      }
    },
    {
      name: 'updated',
      header: 'Updated',
      defaultWidth: 140,
      render: ({ data }) => {
        return (
          <Box
            sx={{
              fontSize: '0.9em',
            }}
          >
            { new Date(data.created).toLocaleString() }
          </Box>
        )
      }
    },
    {
      name: 'type',
      header: 'Type',
      defaultWidth: 120,
      render: ({ data }) => {
        return data.tool_type
      }
    },
    {
      name: 'actions',
      header: '',
      minWidth: 120,
      defaultWidth: 120,
      render: ({ data }) => {
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
                onDelete(data)
              }}
            >
              <DeleteIcon />
            </ClickLink>
            <ClickLink
              onClick={ () => {
                onEdit(data)
              }}
            >
              <EditIcon />
            </ClickLink>
          </Box>
        )
      }
    }]
  }, [
    onEdit,
    onDelete,
    account.token,
  ])

  return (
    <DataGrid2
      autoSort
      userSelect
      rows={ data }
      columns={ columns }
      loading={ false }
    />
  )
}

export default ToolsDataGrid