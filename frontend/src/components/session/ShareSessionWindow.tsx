import { FC, useState } from 'react'

import Window from '../widgets/Window'
import ShareSessionBotForm from './ShareSessionBotForm'
import ShareSessionOptions from './ShareSessionOptions'
import ShareSessionShareForm from './ShareSessionShareForm'

import {
  IBotForm,
  ISession
} from '../../types'

export const ShareSessionWindow: FC<{
  session: ISession,
  onCancel: () => void,
}> = ({
  session,
  onCancel,
}) => {
    // this can be menu, share or bot
    // const initialShowingMode = session.config.original_mode == SESSION_MODE_FINETUNE ? 'menu' : 'share'
    const initialShowingMode = 'share'
    const [optionShowing, setOptionShowing] = useState(initialShowingMode)
    const [bot, setBot] = useState<IBotForm>({
      name: '',
    })

    let content = null

    if (optionShowing == 'menu') {
      content = (
        <ShareSessionOptions
          onShareSession={() => setOptionShowing('share')}
          onPublishBot={() => setOptionShowing('bot')}
        />
      )
    } else if (optionShowing == 'share') {
      content = (
        <ShareSessionShareForm
          session={session}
        />
      )
    } else if (optionShowing == 'bot') {
      content = (
        <ShareSessionBotForm
          bot={bot}
        />
      )
    }

    return (
      <Window
        title="Share"
        size={optionShowing == 'menu' ? 'lg' : 'md'}
        open
        withCancel
        cancelTitle="Close"
        onCancel={onCancel}
        onSubmit={
          optionShowing == 'bot' ?
            () => {

            } :
            undefined
        }
      >
        {content}
      </Window>
    )
  }

export default ShareSessionWindow