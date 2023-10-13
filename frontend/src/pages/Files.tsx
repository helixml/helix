import React, { FC, useContext, useEffect, useState, useCallback, useMemo } from 'react'
import { useQueryParams } from 'hookrouter'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'

import AddIcon from '@mui/icons-material/Add'
import { AccountContext } from '../contexts/account'
import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import FileStoreGrid from '../components/datagrid/FileStore'
import Window from '../components/widgets/Window'

import {
  IFileStoreItem,
} from '../types'

import {
  getRelativePath,
} from '../utils/filestore'

const Files: FC = () => {
  const account = useContext(AccountContext)
  const [ queryParams, setQueryParams ] = useQueryParams() 
  const [ editName, setEditName ] = useState('')

  const sortedFiles = useMemo(() => {
    const folders = account.files.filter((file) => file.directory)
    const files = account.files.filter((file) => !file.directory)
    return folders.concat(files)
  }, [
    account.files,
  ])

  const openFolderEditor = useCallback((id: string) => {
    setQueryParams({
      edit_folder_id: id,
    })
  }, [])

  const onViewFile = useCallback((file: IFileStoreItem) => {
    if(file.directory) {
      account.onSetFilestorePath(getRelativePath(account.filestoreConfig, file))
    } else {
      window.open(file.url)
    }
  }, [
    account.filestoreConfig,
  ])

  const onEditFile = useCallback((file: IFileStoreItem) => {
    
  }, [])

  const onDeleteFile = useCallback((file: IFileStoreItem) => {
    
  }, [])

  useEffect(() => {
    account.onSetFilestorePath(queryParams.path || '/')
    return () => {
      // account.onSetFilestorePath('/')
      // history.replaceState(null, "", location.pathname)
    }
  }, [])

  if(!account.user) return null
  return (
    <>
      <DataGridWithFilters
        filters={
          <Box
            sx={{
              width: '100%',
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
                setQueryParams({
                  edit_folder_id: 'new',
                })
              }}
            >
              Create Folder
            </Button>
          </Box>
        }
        datagrid={
          <FileStoreGrid
            files={ sortedFiles }
            config={ account.filestoreConfig }
            loading={ account.filesLoading }
            onView={ onViewFile }
            onEdit={ onEditFile }
            onDelete={ onDeleteFile }
          />
        }
      />
      {
        queryParams.edit_folder_id && (
          <Window
            open
            title="Edit Folder"
            withCancel
            onCancel={ () => {
              setQueryParams({
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