import React, { FC, useMemo } from 'react'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'

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
        defaultFlex: 1,
        render: ({ data }) => {
          return data.name
        }
      },
      {
        name: 'method',
        header: 'Method',
        defaultFlex: 1,
        render: ({ data }) => {
          return data.method
        }
      },
      {
        name: 'path',
        header: 'Path',
        defaultFlex: 1,
        render: ({ data }) => {
          return data.path
        }
      },
      {
        name: 'description',
        header: 'Description',
        defaultFlex: 1,
        render: ({ data }) => {
          return data.description
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