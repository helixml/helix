import React, { FC, useMemo } from 'react'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
import JsonWindowLink from '../widgets/JsonWindowLink'
import {
  IAssistantGPTScript,
} from '../../types'

const AppGptscriptsDataGrid: FC<React.PropsWithChildren<{
  data: IAssistantGPTScript[],
  onRunScript: (script: IAssistantGPTScript) => void,
}>> = ({
  data,
  onRunScript,
}) => {
  const columns = useMemo<IDataGrid2_Column<IAssistantGPTScript>[]>(() => {
    return [
      {
        name: 'name',
        header: 'Name',
        defaultFlex: 0,
        render: ({ data }) => {
          return <Chip color="secondary" size="small" label={data.name} />
        }
      },
      {
        name: 'path',
        header: 'Path',
        defaultFlex: 0,
        render: ({ data }) => {
          return data.file
        }
      },
      {
        name: 'description',
        header: 'Description',
        defaultFlex: 1,
        render: ({ data }) => {
          return (
            <>
              <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                {data.content?.split('\n').filter(r => r)[0] || ''}
              </Typography>
              <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                {data.content?.split('\n').filter(r => r)[1] || ''}
              </Typography>
              <JsonWindowLink
                sx={{textDecoration: 'underline'}}
                data={data.content}
              >
                expand
              </JsonWindowLink>
            </>
          )
        }
      },
      {
        name: 'actions',
        header: '',
        defaultWidth: 120,
        sx: {
          textAlign: 'right',
        },
        render: ({ data }) => {
          return (
            <Box sx={{
              width: '100%',
              textAlign: 'right',
            }}>
              <Tooltip title="Run Script">
                <IconButton size="small" sx={{ml: 2}} onClick={() => {
                  onRunScript(data)
                }}>
                  <PlayCircleOutlineIcon sx={{width: '32px', height: '32px'}} />
                </IconButton>
              </Tooltip>
            </Box>
          )
        }
      },
    ]
  }, [
    onRunScript,
  ])

  return (
    <DataGrid2
      autoSort
      userSelect
      rows={ data }
      columns={ columns }
      rowHeight={ 70 }
      minHeight={ 300 }
      loading={ false }
    />
  )
}

export default AppGptscriptsDataGrid