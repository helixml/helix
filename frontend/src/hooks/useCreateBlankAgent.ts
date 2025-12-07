import { useCallback } from 'react'
import useApps from './useApps'
import useAccount from './useAccount'
import useSnackbar from './useSnackbar'

/**
 * Hook that provides a function to create a blank agent and navigate to its settings page.
 * Used by both Home.tsx and Apps.tsx to avoid code duplication.
 */
export const useCreateBlankAgent = () => {
  const apps = useApps()
  const account = useAccount()
  const snackbar = useSnackbar()

  const createBlankAgent = useCallback(async () => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }

    try {
      const newAgent = await apps.createAgent({
        name: 'New Agent',
        systemPrompt: '',
        reasoningModelProvider: '',
        reasoningModel: '',
        reasoningModelEffort: '',
        generationModelProvider: '',
        generationModel: '',
        smallReasoningModelProvider: '',
        smallReasoningModel: '',
        smallReasoningModelEffort: '',
        smallGenerationModelProvider: '',
        smallGenerationModel: '',
      })

      if (!newAgent || !newAgent.id) {
        throw new Error('Failed to create agent')
      }

      account.orgNavigate('app', { app_id: newAgent.id })
      snackbar.success('Agent created - configure it below')
    } catch (error) {
      console.error('Error creating agent:', error)
      snackbar.error('Failed to create agent')
    }
  }, [apps, account, snackbar])

  return createBlankAgent
}

export default useCreateBlankAgent
