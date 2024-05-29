import React, { FC, useMemo, useCallback } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import GitHubIcon from '@mui/icons-material/GitHub'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'
import useAccount from '../../hooks/useAccount'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import JsonWindowLink from '../widgets/JsonWindowLink'
import ToolDetail from '../tools/ToolDetail'

import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  IApp,
} from '../../types'

const AppsDataGrid: FC<React.PropsWithChildren<{
  data: IApp[],
  onEdit: (app: IApp) => void,
  onDelete: (app: IApp) => void,
}>> = ({
  data,
  onEdit,
  onDelete,
}) => {

  const theme = useTheme()
  const account = useAccount()

  const tableData = useMemo(() => {
    return data.map(app => {
      const gptScripts = (app.config.helix?.assistants[0]?.gptscripts || [])
      const apiTools = (app.config.helix?.assistants[0]?.tools || []).filter(t => t.tool_type == 'api')

      const gptscriptsElem = gptScripts.length > 0 ? (
        <>
          <Box sx={{mb: 2}}>
            <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
              GPTScripts
            </Typography>
          </Box>
          {
            // TODO: support more than 1 assistant
            gptScripts.map((gptscript, index) => {
              return (
                <Box
                  key={index}
                >
                  <Row>
                    <Cell sx={{width:'50%'}}>
                      <Chip color="secondary" size="small" label={gptscript.name} />
                    </Cell>
                    <Cell sx={{width:'50%'}}>
                      <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                        {gptscript.content?.split('\n').filter(r => r)[0] || ''}
                      </Typography>
                      <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                        {gptscript.content?.split('\n').filter(r => r)[1] || ''}
                      </Typography>
                      <JsonWindowLink
                        sx={{textDecoration: 'underline'}}
                        data={gptscript.content}
                      >
                        expand
                      </JsonWindowLink>
                    </Cell>
                  </Row>
                </Box>
              )
            })
          }
        </>
      ) : null

      const apisElem = apiTools.length > 0 ? (
        <>
          <Box sx={{mb: 2}}>
            <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
              APIs
            </Typography>
          </Box>
          {
            // TODO: support more than 1 assistant
            apiTools.map((apiTool, index) => {
              return (
                <ToolDetail
                  key={ index }
                  tool={ apiTool }
                />
              )
            })
          }
        </>
      ) : null

      // TODO: add the list of apis also to this display
      // we can grab stuff from the tools package that already renders endpoints

      const accessType = app.global ? 'Global' : 'User'

      return {
        id: app.id,
        _data: app,
        name: (
          <Row>
            <Cell sx={{pr: 2,}}>
              <GitHubIcon />
            </Cell>
            <Cell grow>
              <Typography variant="body1">
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
                    onEdit(app)
                  }}
                >
                  { app.config.helix.name }
                </a>
              </Typography>
              <Typography variant="caption">
                { app.config.github?.repo }
              </Typography>
            </Cell>
          </Row>
        ),
        type: (
          <Typography variant="body1">
            { app.app_source } ({accessType})
          </Typography>
        ),
        details: (
          <>
            { gptscriptsElem }
            { apisElem }
          </>
        ),
        updated: (
          <Box
            sx={{
              fontSize: '0.9em',
            }}
          >
            { new Date(app.updated).toLocaleString() }
          </Box>
        ),
      }
    })
  }, [
    theme,
    data,
  ])

  const getActions = useCallback((app: any) => {
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
            onDelete(app._data)
          }}
        >
          <Tooltip title="Delete">
            <DeleteIcon />
          </Tooltip>
        </ClickLink>
      
        <ClickLink
          onClick={ () => {
            onEdit(app._data)
          }}
        >
          <Tooltip title="Edit">
            <EditIcon />
          </Tooltip>
        </ClickLink>
      </Box>
    )
  }, [

  ])

  return (
    <SimpleTable
      fields={[{
        name: 'name',
        title: 'Name',
      }, {
        name: 'type',
        title: 'Type',
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

export default AppsDataGrid