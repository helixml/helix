import {
  IInteraction,
  SESSION_CREATOR_SYSTEM,
} from '../types'

export const getSystemMessage = (message: string): IInteraction => {
  return {
    id: 'system',
    created: 0,
    creator: SESSION_CREATOR_SYSTEM,
    runner: '',
    finetune_file: '',
    error: '',
    metadata: {},
    message,
    progress: 0,
    files: [],
    finished: true,
  }
}