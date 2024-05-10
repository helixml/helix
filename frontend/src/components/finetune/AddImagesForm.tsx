import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import FileUploadArea from './FileUploadArea'
import FileList from './FileList'

import {
  IUploadFile,
} from '../../types'

export const AddImagesForm: FC<{
  files: IUploadFile[],
  onAddFiles: (files: IUploadFile[]) => void,
}> = ({
  files,
  onAddFiles,
}) => {
  return (
    <>
      <Typography
        sx={{
          fontWeight: 'bold',
          mt: 3,
          mb: 3,
        }}
        className="interactionMessage"
      >
        Upload some images you want your model to learn from
      </Typography>
      <FileUploadArea
        onlyImages
        height={ 200 }
        onAddFiles={ onAddFiles }
      />
      <FileList
        files={ files }
      />
    </>
  )
}

export default AddImagesForm