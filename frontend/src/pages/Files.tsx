import React, { FC, useState, useCallback, useMemo, Fragment } from 'react'
import prettyBytes from 'pretty-bytes'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import AddIcon from '@mui/icons-material/Add'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'

import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import FileStoreGrid from '../components/datagrid/FileStore'
import Window from '../components/widgets/Window'
import FileUpload from '../components/widgets/FileUpload'
import Progress from '../components/widgets/Progress'
import ClickLink from '../components/widgets/ClickLink'

import useFilestore from '../hooks/useFilestore'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'

import {
  IFileStoreItem,
} from '../types'

import {
  getRelativePath,
} from '../utils/filestore'

const Files: FC = () => {
  const account = useAccount()
  const filestore = useFilestore()
  
  const {
    name,
    params,
    setParams,
  } = useRouter()

  const [ editName, setEditName ] = useState('')

  const sortedFiles = useMemo(() => {
    const folders = filestore.files.filter((file) => file.directory)
    const files = filestore.files.filter((file) => !file.directory)
    return folders.concat(files)
  }, [
    filestore.files,
  ])

  const openFolderEditor = useCallback((id: string) => {
    setParams({
      edit_folder_id: id,
    })
  }, [
    setParams,
  ])

  const onUpload = useCallback(async (files: File[]) => {
    await filestore.onUpload(filestore.path, files)
  }, [
    filestore.path,
  ])

  const onViewFile = useCallback((file: IFileStoreItem) => {
    if(file.directory) {
      filestore.setPath(getRelativePath(filestore.config, file))
    } else {
      window.open(file.url)
    }
  }, [
    filestore.config,
  ])

  const onEditFile = useCallback((file: IFileStoreItem) => {
    
  }, [])

  const onDeleteFile = useCallback((file: IFileStoreItem) => {
    
  }, [])

  if(!account.user) return null
  return (
    <>
      <Box
        sx={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
        }}
      >
        <Box
          sx={{
            flexGrow: 0,
            pl: 6,
            width: '100%',
            height: '60px',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
          }}
        >
          {
            filestore.breadcrumbs.map((breadcrumb, i) => {
              return (
                <Fragment key={ i }>
                  <ClickLink
                    textDecoration
                    key={ i }
                    onClick={ () => {
                      filestore.setPath(breadcrumb.path)
                    }}
                  >
                    { breadcrumb.title }
                  </ClickLink>
                  {
                    i < filestore.breadcrumbs.length - 1 && (
                      <Box
                        sx={{
                          ml: 1,
                          mr: 1,
                        }}
                      >
                        :
                      </Box>
                    )
                  }
                </Fragment>
              )
            })
          }
        </Box>
        <Box
          sx={{
            width: '100%',
            flexGrow: 1,
          }}
        >
          <DataGridWithFilters
            filters={
              <Box
                sx={{
                  width: '100%',
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                }}
              >
                {
                  filestore.uploadProgress ? (
                    <>
                      <Typography
                        sx={{
                          mb: 2,
                        }}
                        variant="caption"
                      >
                        uploaded { prettyBytes(filestore.uploadProgress.uploadedBytes) } of { prettyBytes(filestore.uploadProgress.totalBytes) }
                      </Typography>
                      <Progress progress={ filestore.uploadProgress.percent } />
                    </>
                  ) : (
                    <>
                      <Button
                        sx={{
                          width: '100%',
                        }}
                        variant="contained"
                        color="secondary"
                        endIcon={<AddIcon />}
                        onClick={ () => {
                          setParams({
                            edit_folder_id: 'new',
                          })
                        }}
                      >
                        Create Folder
                      </Button>
                      <FileUpload
                        sx={{
                          width: '100%',
                          mt: 2,
                        }}
                        onUpload={ onUpload }
                      >
                        <Button
                          sx={{
                            width: '100%',
                          }}
                          variant="contained"
                          color="secondary"
                          endIcon={<CloudUploadIcon />}
                        >
                          Upload Files
                        </Button>
                        <Box
                          sx={{
                            border: '1px dashed #ccc',
                            p: 2,
                            display: 'flex',
                            flexDirection: 'row',
                            alignItems: 'center',
                            justifyContent: 'center',
                            minHeight: '100px',
                            cursor: 'pointer',
                          }}
                        >
                          <Typography
                            sx={{
                              color: '#999'
                            }}
                            variant="caption"
                          >
                            drop files here to upload them...
                          </Typography>
                        </Box>
                      </FileUpload>
                    </>
                  )
                }
              </Box>
            }
            datagrid={
              <FileStoreGrid
                files={ sortedFiles }
                config={ filestore.config }
                loading={ filestore.loading || filestore.uploadProgress ? true : false }
                onView={ onViewFile }
                onEdit={ onEditFile }
                onDelete={ onDeleteFile }
              />
            }
          />
        </Box>
      </Box>
      {
        params.edit_folder_id && (
          <Window
            open
            title="Edit Folder"
            withCancel
            onCancel={ () => {
              setParams({
                edit_folder_id: ''
              }) 
            }}
            onSubmit={ () => {
              console.log('--------------------------------------------')
            }}
          >
            <Box
              sx={{
                p: 2,
              }}
            >
              <TextField
                fullWidth
                label="Folder Name"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
              />
            </Box>
          </Window>
        )
      }
    </>
  )
}

export default Files