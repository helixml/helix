import React, { FC } from 'react'
import VisibilityIcon from '@mui/icons-material/Visibility'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import FolderIcon from '@mui/icons-material/Folder'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import prettyBytes from 'pretty-bytes'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import ClickLink from '../widgets/ClickLink'

import {
  IFileStoreItem,
} from '../../types'

const getFileExtension = (filename: string) => {
  const parts = filename.split('.')
  return parts[parts.length - 1]
}

const isImage = (filename: string) => {
  return false
}

const isMedia = (filename: string) => {
  return false
}

const columns: IDataGrid2_Column<IFileStoreItem>[] = [
  {
    name: 'icon',
    header: '',
    defaultWidth: 100,
    render: ({ data }) => {
      let icon = null

      if(isImage(data.name) && isMedia(data.name)) {
        icon = (
          <div>
            <Box
              component={'img'}
              sx={{
                maxWidth: '80px',
                maxHeight: '80px',
                border: '1px solid',
                borderColor: 'secondary.main',
              }}
              src={ data.url }
            />
          </div>
        )
      }
      else if(data.directory) {
        icon = (
          <Avatar>
            <FolderIcon />
          </Avatar>
        )
      }
      else {
        icon = (
          <span className={`fiv-viv fiv-size-md fiv-icon-${getFileExtension(data.name)}`}></span>
        )
      }

      return icon
    }
  },
  {
    name: 'name',
    header: 'Name',
    defaultFlex: 1,
    render: ({ data }) => {
      return data.directory ? (
        <a href="#" onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
          e.preventDefault()
          e.stopPropagation()
          console.log('--------------------------------------------')
        }}>
          { data.name }
        </a>
      ) : (
        <a href={ data.url } target="_blank">
          { data.name }
        </a>
      )
    }
  },
  {
    name: 'updated',
    header: 'Updated',
    defaultFlex: 1,
    render: ({ data }) => {
      return (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          { new Date(data.created * 1000).toLocaleString() }
        </Box>
      )
    }
  },
  {
    name: 'size',
    header: 'Size',
    defaultFlex: 1,
    render: ({ data }) => {
      return data.directory ? null : (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          { prettyBytes(data.size) }
        </Box>
      )
    }
  },
  {
    name: 'actions',
    header: 'Actions',
    minWidth: 100,
    defaultWidth: 100,
    textAlign: 'end',
    render: ({ data }) => {
      return (
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
          }}
        >
          <ClickLink
            onClick={ () => {
              console.log('--------------------------------------------')
            }}
          >
            <DeleteIcon />
          </ClickLink>
          <ClickLink
            onClick={ () => {
              console.log('--------------------------------------------')
            }}
          >
            <EditIcon />
          </ClickLink>
          <ClickLink
            onClick={ () => {
              console.log('--------------------------------------------')
            }}
          >
            <VisibilityIcon />
          </ClickLink>
        </Box>
      )
    }
  },
]

interface FileStoreDataGridProps {
  files: IFileStoreItem[],
  loading: boolean,
}

const FileStoreDataGrid: FC<React.PropsWithChildren<FileStoreDataGridProps>> = ({
  files,
  loading,
}) => {

  return (
    <DataGrid2
      autoSort
      userSelect
      rows={ files }
      columns={ columns }
      loading={ loading }
    />
  )
}

export default FileStoreDataGrid