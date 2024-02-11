import React, { FC, useMemo } from 'react'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'

import {
  IToolApiAction,
} from '../../types'

const ToolActionsDataGrid: FC<React.PropsWithChildren<{
  data: IToolApiAction[],
}>> = ({
  data,
}) => {
  const columns = useMemo<IDataGrid2_Column<IToolApiAction>[]>(() => {
    return [
      {
        name: 'name',
        header: 'Name',
        defaultFlex: 0,
        render: ({ data }) => {
          return data.name
        }
      },
      {
        name: 'method',
        header: 'Method',
        defaultFlex: 0,
        render: ({ data }) => {
          return data.method
        }
      },
      {
        name: 'path',
        header: 'Path',
        defaultFlex: 0,
        render: ({ data }) => {
          return data.path
        }
      },
      {
        name: 'description',
        header: 'Description',
        defaultFlex: 1,
        render: ({ data }) => {
          return (
            <Typography sx={{ fontSize: '0.7rem', whiteSpace: 'normal', wordWrap: 'break-word', overflowWrap: 'break-word' }}>
              {data.description}
            </Typography>
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
      loading={ false }
    />
  )
}

export default ToolActionsDataGrid