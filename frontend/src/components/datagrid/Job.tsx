import React, { FC } from 'react'
import VisibilityIcon from '@mui/icons-material/Visibility'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import JsonWindowLink from '../widgets/JsonWindowLink'

import {
  IJob,
} from '../../types'

const columns: IDataGrid2_Column<IJob>[] = [
  {
    name: 'created_at',
    header: 'Date',
    defaultFlex: 1,
    render: ({ data }) => {
      return (
        <div>{ new Date(data.created).toLocaleString() }</div>
      )
    }
  },
  {
    name: 'id',
    header: 'ID',
    defaultFlex: 1,
  },
  {
    name: 'state',
    header: 'State',
    defaultFlex: 1,
  },
  {
    name: 'actions',
    header: 'Actions',
    minWidth: 100,
    defaultWidth: 100,
    textAlign: 'end',
    render: ({ data }) => {
      return (
        <JsonWindowLink
          data={ data }
        >
          <VisibilityIcon />
        </JsonWindowLink>
      )
    }
  },
]

interface JobDataGridProps {
  jobs: IJob[],
  loading: boolean,
}

const JobDataGrid: FC<React.PropsWithChildren<JobDataGridProps>> = ({
  jobs,
  loading,
}) => {

  return (
    <DataGrid2
      autoSort
      userSelect
      rows={ jobs }
      columns={ columns }
      loading={ loading }
    />
  )
}

export default JobDataGrid