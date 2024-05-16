import React, { FC } from 'react'
import SelectOption from '../widgets/SelectOption'

import {
  ISessionMode,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
} from '../../types'

import {
  MODE_LABELS,
} from '../../config'

const SessionModeDropdown: FC<{
  mode: ISessionMode,
  cellWidth?: number,
  onSetMode: (mode: ISessionMode) => void,
}> = ({
  mode,
  cellWidth,
  onSetMode,
}) => {
  return (
    <SelectOption
      value={MODE_LABELS[mode]}
      options={[MODE_LABELS[SESSION_MODE_INFERENCE], MODE_LABELS[SESSION_MODE_FINETUNE]]}
      onSetValue={mode => {
        const useMode = [SESSION_MODE_INFERENCE, SESSION_MODE_FINETUNE].find(m => MODE_LABELS[m] == mode)
        onSetMode(useMode as ISessionMode) 
      }}
    />
  )
}

export default SessionModeDropdown
