import React, { FC, useMemo, useCallback } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import ViewIcon from '@mui/icons-material/Visibility'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Stack from '@mui/material/Stack'
import Chip from '@mui/material/Chip'
import SimpleTable from '../widgets/SimpleTable'
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

  const isAdmin = account.admin

  const tableData = useMemo(() => {
    return data.map(tool => {
      const accessType = tool.global ? 'Global' : 'User'
      const toolType = tool.config.gptscript ? 'GPT Script' : 'Tool'
      let details: any = ''
      if(tool.config.api) {
        details = (
          <>
            <Box sx={{mb: 2}}>
              <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
                { tool.config.api.url }
              </Typography>
              <Typography variant="caption" gutterBottom>
                { tool.description }
              </Typography>
            </Box>
            {
              tool.config.api.actions.map((action, index) => {
                return (
                  <Box key={index}>
                    <Row>
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
                    <Row sx={{mt: 0.5, mb: 2}}>
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
      if(tool.config.gptscript) {
        details = (
          <>
            <Box sx={{mb: 2}}>
              {
                tool.config.gptscript.script_url && (
                  <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
                    { tool.config.gptscript.script_url }
                  </Typography>
                )
              }
              <Typography variant="caption" gutterBottom>
                { tool.description }
              </Typography>
            </Box>
          </>
        )
      }
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
            { new Date(tool.created).toLocaleString() }
          </Box>
        ),
        details,
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