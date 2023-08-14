import React, {FC,useCallback} from 'react'
import {useDropzone} from 'react-dropzone'

const FileUpload: FC<{
  
}> = ({
   
}) => {
  const onDrop = useCallback(acceptedFiles => {
    
  }, [])
  const {getRootProps, getInputProps, isDragActive} = useDropzone({onDrop})

  return (
    <div {...getRootProps()}>
      <input {...getInputProps()} />
      {
        isDragActive ?
          <p>Drop the files here ...</p> :
          <p>Drag 'n' drop some files here, or click to select files</p>
      }
    </div>
  )
}

export default FileUpload