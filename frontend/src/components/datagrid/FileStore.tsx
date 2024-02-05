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
  IFileStoreItem,
  IFileStoreConfig,
} from '../../types'

import {
  mapFileExtension,
  isImage,
} from '../../utils/filestore'

interface FileStoreDataGridProps {
  files: IFileStoreItem[],
  config: IFileStoreConfig,
  loading: boolean,
  readonly: boolean,
  onView: (file: IFileStoreItem) => void,
  onEdit: (file: IFileStoreItem) => void,
  onDelete: (file: IFileStoreItem) => void,
}

const FileStoreDataGrid: FC<React.PropsWithChildren<FileStoreDataGridProps>> = ({
  files,
  config,
  loading,
  readonly,
  onView,
  onEdit,
  onDelete,
}) => {

  const account = useAccount()

  const columns = useMemo<IDataGrid2_Column<IFileStoreItem>[]>(() => {
    return [{
      name: 'icon',
      header: '',
      defaultWidth: 100,
      render: ({ data }) => {
        let icon = null
  
        if(isImage(data.name)) {
          icon = account.token ? (
            <Box
              component={'img'}
              sx={{
                maxWidth: '50px',
                maxHeight: '50px',
                border: '1px solid',
                borderColor: 'secondary.main',
              }}
              src={ `${data.url}?access_token=${account.token}` }
            />
          ) : null
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
            <span className={`fiv-viv fiv-size-md fiv-icon-${mapFileExtension(data.name)}`}></span>
          )
        }
  
        return (
          <Box
            sx={{
              width: '100%',
              height: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <ClickLink
              onClick={ () => {
                onView(data)
              }}
              sx={{
                textDecoration: 'none',
                color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
              }}
            >
              { icon }
            </ClickLink>
            
          </Box>
      )
      }
    },
    {
      name: 'name',
      header: 'Name',
      defaultFlex: 1,
      render: ({ data }) => {
        return (
          <a style={{ textDecoration: 'none', color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary, }} href="#" onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
            e.preventDefault()
            e.stopPropagation()
            onView(data)
          }}>
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
            { new Date(data.created * 1000).toLocaleString() }
          </Box>
        )
      }
    },
    {
      name: 'size',
      header: 'Size',
      defaultWidth: 120,
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
      minWidth: 160,
      defaultWidth: 160,
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
            {
              readonly ? null : (
                <>
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
                </>
              )
            }
            <ClickLink
              onClick={ () => {
                onView(data)
              }}
            >
              <VisibilityIcon />
            </ClickLink>
          </Box>
        )
      }
    }]
  }, [
    readonly,
    onView,
    onEdit,
    onDelete,
    account.token,
  ])

  const theme = useTheme()
  const themeConfig = useThemeConfig()

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