import React, { FC, useMemo } from 'react'

import Window from '../widgets/Window'
import FineTuneImageInputs from './FineTuneImageInputs'
import FineTuneImageLabels from './FineTuneImageLabels'
import FineTuneTextInputs from './FineTuneTextInputs'
import UploadingOverlay from '../widgets/UploadingOverlay'

import useCreateInputs from '../../hooks/useCreateInputs'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'

import {
  ISession,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const AddFilesWindow: FC<{
  session: ISession,
  // if this is defined it means "add the files to this specific interaction"
  interactionID?: string,
 onClose: (filesAdded: boolean) => void,
  }> = ({
    session,
    interactionID,
    onClose,
  }) => {
  const snackbar = useSnackbar()
  const api = useApi()
  const inputs = useCreateInputs()

  const addDocumentsSubmitTitle = useMemo(() => {
    if(session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 0) {
      return "Next Step"
    } else {
      return "Upload"
    }
  }, [
    session.type,
    inputs.fineTuneStep,
  ])

  // this is for text finetune
  const onAddDocuments = async () => {
    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })

    try {
      const formData = inputs.getFormData(session.mode, session.type, session.model_name)
      await api.put(`/api/v1/sessions/${session.id}/finetune/documents`, formData, {
        onUploadProgress: inputs.uploadProgressHandler,
        params: {
          interactionID: interactionID || '',
        }
      })
      if(!session) {
        inputs.setUploadProgress(undefined)
        return
      }
      snackbar.success('Documents added...')
      onClose(true)
      return
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  // this is for image finetune
  const onAddImageDocuments = async () => {
    // TODO: sort this
    // const errorFiles = inputs.files.filter(file => inputs.labels[file.name] ? false : true)
    // if(errorFiles.length > 0) {
    //   inputs.setShowImageLabelErrors(true)
    //   snackbar.error('Please add a label to each image')
    //   return
    // }
    // inputs.setShowImageLabelErrors(false)
    // onAddDocuments()
  }

  return (
    <>
      <Window
        open
        size="lg"
        // title={`Add Documents to ${session.name}?`}
        withCancel
        cancelTitle="Cancel" 
        submitTitle={ addDocumentsSubmitTitle }
        onSubmit={ () => {
          if(session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 0) {
            inputs.setFineTuneStep(1)
          } else if(session.type == SESSION_TYPE_TEXT && inputs.fineTuneStep == 0) {
            onAddDocuments()
          } else if(session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 1) {
            onAddImageDocuments()
          }
        }}
        onCancel={ () => onClose(false) }
      >
        {
          session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 0 && (
            <FineTuneImageInputs
              initialFiles={ inputs.finetuneFiles.map(f => f.file) }
              // showAssistantInteraction={ false }
              onChange={ (files) => {
                // TODO: sort this
                //inputs.setFinetuneFiles(files)
              }}
            />
          )
        }
        {
          session.type == SESSION_TYPE_TEXT && inputs.fineTuneStep == 0 && (
            <FineTuneTextInputs
              initialCounter={ inputs.manualTextFileCounter }
              initialFiles={ inputs.finetuneFiles.map(f => f.file) }
              showAssistantInteraction={ false }
              onChange={ (counter, files) => {
                // TODO: sort this
                // inputs.setManualTextFileCounter(counter)
                // inputs.setFinetuneFiles(files)
              }}
            />
          )
        }
        {
          session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 1 && (
            <FineTuneImageLabels
              showImageLabelErrors={ inputs.showImageLabelErrors }
              initialLabels={ inputs.labels }
              showAssistantInteraction={ false }
              files={ inputs.finetuneFiles.map(f => f.file) }
              onChange={ (labels) => {
                inputs.setLabels(labels)
              }}
            />
          )
        }
      </Window>
      {
        inputs.uploadProgress && (
          <UploadingOverlay
            percent={ inputs.uploadProgress.percent }
          />
        )
      }
    </>
  )
}

export default AddFilesWindow