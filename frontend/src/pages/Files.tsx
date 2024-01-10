import React, { FC, useState, useCallback, useMemo, Fragment } from 'react'
import { prettyBytes } from '../utils/format'
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
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useFilestore from '../hooks/useFilestore'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import {
  IFileStoreItem,
} from '../types'

import {
  getRelativePath,
} from '../utils/filestore'

const Files: FC = () => {
  const account = useAccount()
  const filestore = useFilestore()
  const snackbar = useSnackbar()
  
  const {
    name,
    params,
    setParams,
    removeParams,
  } = useRouter()

  const {
    // this is actually the "name" of the file / folder
    edit_id,
    edit_item_title,
    delete_id,
    delete_item_title,
  } = params
  
  const [ editName, setEditName ] = useState('')

  const sortedFiles = useMemo(() => {
    const folders = filestore.files.filter((file) => file.directory)
    const files = filestore.files.filter((file) => !file.directory)
    return folders.concat(files)
  }, [
    filestore.files,
  ])

  const onUpload = useCallback(async (files: File[]) => {
    const result = await filestore.upload(filestore.path, files)
    if(!result) return
    await filestore.loadFiles(filestore.path)
    snackbar.success('Files Uploaded')
  }, [
    filestore.path,
  ])

  const onViewFile = useCallback((file: IFileStoreItem) => {
    if(!account.user) {
      snackbar.error('must be logged in')
      return 
    }
    if(file.directory) {
      filestore.setPath(getRelativePath(filestore.config, file))
    } else {
      window.open(`${file.url}?access_token=${account.user.token}`)
    }
  }, [
    filestore.config,
    account.user,
  ])

  const onEditFile = useCallback((file: IFileStoreItem) => {
    setEditName(file.name)
    setParams({
      edit_id: file.name,
      edit_item_title: file.directory ? 'Directory' : 'File',
    })
  }, [
    setParams,
  ])

  const onDeleteFile = useCallback(async (file: IFileStoreItem) => {
    setParams({
      delete_id: file.name,
      delete_item_title: file.name,
    })
  }, [
    filestore.path,
  ])

  const onSubmitEditWindow = useCallback(async (newName: string) => {
    let result = false
    let message = ''
    if(edit_id == 'new_folder') {
      result = await filestore.createFolder(newName)
      message = 'Folder Created'
    } else {
      result = await filestore.rename(edit_id, newName)
      message = `${edit_item_title} Renamed`
    }
    if(!result) return
    await filestore.loadFiles(filestore.path)
    snackbar.success(message)
    removeParams(['edit_item_title', 'edit_id'])
  }, [
    edit_id,
    edit_item_title,
    filestore.path,
  ])

  const onConfirmDelete = useCallback(async () => {
    const result = await filestore.del(delete_id)
    if(!result) return
    await filestore.loadFiles(filestore.path)
    snackbar.success(`${delete_item_title} Deleted`)
    removeParams(['delete_item_title', 'delete_id'])
  }, [
    delete_id,
    delete_item_title,
    filestore.path,
  ])

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
                  ) : filestore.readonly ? null : (
                    <>
                      <Button
                        sx={{
                          width: '100%',
                        }}
                        variant="contained"
                        color="secondary"
                        endIcon={<AddIcon />}
                        onClick={ () => {
                          setEditName('')
                          setParams({
                            edit_item_title: 'Folder',
                            edit_id: 'new_folder',
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
                readonly={ filestore.readonly }
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
        edit_id && (
          <Window
            open
            title={ `${edit_id == 'new_folder' ? 'New' : 'Edit'} ${edit_item_title}` }
            withCancel
            onCancel={ () => removeParams(['edit_item_title', 'edit_id']) }
            onSubmit={ () => onSubmitEditWindow(editName) }
          >
            <Box
              sx={{
                p: 2,
              }}
            >
              <TextField
                fullWidth
                label={ `${edit_item_title} Name` }
                value={ editName }
                onChange={(e) => setEditName(e.target.value)}
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    onSubmitEditWindow(editName)
                  }
                }}
              />
            </Box>
          </Window>
        )
      }
      {
        delete_id && (
          <DeleteConfirmWindow
            title={ delete_item_title }
            onCancel={ () => removeParams(['delete_item_title', 'delete_id']) }
            onSubmit={ onConfirmDelete }
          />
        )
      }
    </>
  )
}

export default Files