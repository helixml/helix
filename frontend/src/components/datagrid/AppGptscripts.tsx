import React, { FC, useMemo } from 'react'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import JsonWindowLink from '../widgets/JsonWindowLink'
import {
  IAppHelixConfigGptScript,
} from '../../types'

const AppGptscriptsDataGrid: FC<React.PropsWithChildren<{
  data: IAppHelixConfigGptScript[],
}>> = ({
  data,
}) => {
  const columns = useMemo<IDataGrid2_Column<IAppHelixConfigGptScript>[]>(() => {
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
          return data.file_path
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
    ]
  }, [])

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