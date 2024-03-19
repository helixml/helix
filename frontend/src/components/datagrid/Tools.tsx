import React, { FC, useMemo } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Stack from '@mui/material/Stack'
import Chip from '@mui/material/Chip'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import ClickLink from '../widgets/ClickLink'
import useAccount from '../../hooks/useAccount'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

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

  const columns = useMemo<IDataGrid2_Column<ITool>[]>(() => {
    return [
    {
      name: 'name',
      header: 'Name',
      defaultFlex: 0,
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
      name: 'type',
      header: 'Type',
      defaultFlex: 0,
      render: ({ data }) => {
        return data.global ? 'Global' : 'User'
      }
    },
    {
      name: 'updated',
      header: 'Updated',
      defaultFlex: 0,
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
      name: 'url',
      header: 'URL',
      minWidth: 240,
      defaultWidth: 240,
      render: ({ data }) => {
        return data.config.api.url
      }
    },
    {
      name: 'actions',
      header: 'Actions',
      defaultFlex: 1,
      render: ({ data }) => {
        return (
          <>
          {
            data.config.api.actions.map((action, index) => {
              return (
                <Box key={index}>
                  <Row sx={{mt:1}}>
                    <Cell sx={{width:'50%'}}>
                      <Typography>
                        {action.name}
                      </Typography>
                    </Cell>
                    <Cell sx={{width:'50%'}}>
                      <Row>
                        <Cell sx={{width: '70px'}}>
                          <Chip color="secondary" size="small" label={action.method.toUpperCase()} />
                        </Cell>
                        <Cell>
                          <Typography>
                            {action.path}
                          </Typography>
                        </Cell>
                      </Row>
                    </Cell>
                  </Row>
                  <Row>
                    <Cell>
                      <Typography variant="caption" sx={{color: '#999'}}>
                        {action.description}
                      </Typography>
                    </Cell>
                  </Row>
                </Box>
              )
            })
          }
          </>
        )
      }
    },
    {
      name: 'actionbuttons',
      header: '',
      minWidth: 120,
      defaultWidth: 120,
      render: ({ data }) => {
        if(data.global && !account.admin) return null
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
    account.admin,
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