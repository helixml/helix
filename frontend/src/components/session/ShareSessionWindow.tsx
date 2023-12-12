import React, { FC, useState, useMemo } from 'react'

import Window from '../widgets/Window'
import ShareSessionOptions from './ShareSessionOptions'
import ShareSessionShareForm from './ShareSessionShareForm'
import ShareSessionBotForm from './ShareSessionBotForm'

import {
  ISession,
  IBotForm,
  SESSION_MODE_FINETUNE,
} from '../../types'

export const ShareSessionWindow: FC<{
  session: ISession,
  onUpdateSharing: (value: boolean) => Promise<boolean>,
  onShare: () => Promise<boolean>,
  onCancel: () => void,
}> = ({
  session,
  onUpdateSharing,
  onShare,
  onCancel,
}) => {
  // this can be menu, share or bot
  const [ optionShowing, setOptionShowing ] = useState(session.config.original_mode == SESSION_MODE_FINETUNE ? 'menu' : 'share')
  const [ shared, setShared ] = useState(session.config.shared ? true : false)
  const [ bot, setBot ] = useState<IBotForm>({
    name: '',
  })

  let content = null

  if(optionShowing == 'menu') {
    content = (
      <ShareSessionOptions
        onShareSession={ () => setOptionShowing('share') }
        onPublishBot={ () => setOptionShowing('bot') }
      />
    )
  } else if(optionShowing == 'share') {
    content = (
      <ShareSessionShareForm
        session={ session }
        shared={ shared }
        onChange={ (val) => {
          setShared(val)
          onUpdateSharing(val)
        }}
      />
    )
  } else if(optionShowing == 'bot') {
    content = (
      <ShareSessionBotForm
        bot={ bot }
        onChange={ setBot }
      />
    )
  }

  return (
    <Window
      title="Share"
      size={ optionShowing == 'menu' ? 'lg' : 'md' }
      open
      withCancel
      cancelTitle="Close"
      onCancel={ onCancel }
      onSubmit={
        optionShowing == 'bot' ?
          () => {

          } :
          undefined
      }
    >
      { content } 
    </Window>
  )  
}

export default ShareSessionWindow