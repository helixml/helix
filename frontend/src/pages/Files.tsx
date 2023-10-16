import React, { FC, useState, useCallback, useMemo } from 'react'
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
      filestore.onSetPath(getRelativePath(filestore.config, file))
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
          This is the header
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
              </Box>
            }
            datagrid={
              <FileStoreGrid
                files={ sortedFiles }
                config={ filestore.config }
                loading={ filestore.loading }
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