import React, {FC,useCallback} from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'
import { useDropzone, DropzoneOptions } from 'react-dropzone'

const FileUpload: FC<{
  sx?: SxProps,
  onlyImages?: boolean,
  onUpload: (files: File[]) => Promise<void>,
}> = ({
  children,
  sx = {},
  onlyImages = false,
  onUpload,
}) => {
  const opts: DropzoneOptions = {
    onDrop: onUpload,
  }
  if(onlyImages) {
    opts.accept = {
      'image/jpeg': [],
      'image/png': [],
      'image/gif': [],
    }
  }
  const {getRootProps, getInputProps, isDragActive} = useDropzone(opts)

  return (
    <Box {...getRootProps()} sx={ sx }>
      <input {...getInputProps()} />
      {
        children
      }
    </Box>
  )
}

export default FileUpload