import React, { FC, useMemo } from 'react'

import Window from '../widgets/Window'
import FineTuneImageInputs from './FineTuneImageInputs'
import FineTuneImageLabels from './FineTuneImageLabels'
import FineTuneTextInputs from './FineTuneTextInputs'

import { IFinetuneInputs } from '../../hooks/useFinetuneInputs'

import {
  ISession,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const AddMoreFiles: FC<{
  session: ISession,
  sessionID: string,
  interactionID: string,
  inputs: IFinetuneInputs,
  onSubmit: () => Promise<boolean>,
  onCancel: () => void,
}> = ({
  session,
  inputs,
  onSubmit,
  onCancel,
}) => {
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

  return (
    <>
      <Window
        open
        size="lg"
        title={`Add Documents to ${session.name}?`}
        withCancel
        submitTitle={ addDocumentsSubmitTitle }
        onSubmit={ onSubmit }
        onCancel={ onCancel }
      >
        {
          session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 0 && (
            <FineTuneImageInputs
              initialFiles={ inputs.files }
              onChange={ (files) => {
                inputs.setFiles(files)
              }}
            />
          )
        }
        {
          session.type == SESSION_TYPE_TEXT && inputs.fineTuneStep == 0 && (
            <FineTuneTextInputs
              initialCounter={ inputs.manualTextFileCounter }
              initialFiles={ inputs.files }
              onChange={ (counter, files) => {
                inputs.setManualTextFileCounter(counter)
                inputs.setFiles(files)
              }}
            />
          )
        }
        {
          session.type == SESSION_TYPE_IMAGE && inputs.fineTuneStep == 1 && (
            <FineTuneImageLabels
              showImageLabelErrors={ inputs.showImageLabelErrors }
              initialLabels={ inputs.labels }
              files={ inputs.files }
              onChange={ (labels) => {
                inputs.setLabels(labels)
              }}
            />
          )
        }
      </Window>
    </>
  )
}

export default AddMoreFiles