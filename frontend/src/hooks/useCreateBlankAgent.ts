import { useCallback, useRef } from 'react'
import useApps from './useApps'
import useAccount from './useAccount'
import useSnackbar from './useSnackbar'

/**
 * Hook that provides a function to create a blank agent and navigate to its settings page.
 * Used by both Home.tsx and Apps.tsx to avoid code duplication.
 * Creates the agent with no model pre-selected — the user configures it on the settings page.
 */
export const useCreateBlankAgent = () => {
  const apps = useApps()
  const account = useAccount()
  const snackbar = useSnackbar()
  const creatingRef = useRef(false)

  const createBlankAgent = useCallback(async () => {
    if (creatingRef.current) return
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }

    creatingRef.current = true
    try {
      const newAgent = await apps.createAgent({
        name: 'New Agent',
        systemPrompt: '',
        model: '',
        provider: '',
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

      account.orgNavigate('agent', { app_id: newAgent.id })
      snackbar.success('Agent created - configure it below')
    } catch (error) {
      console.error('Error creating agent:', error)
      snackbar.error('Failed to create agent')
    } finally {
      creatingRef.current = false
    }
  }, [apps, account, snackbar])

  return createBlankAgent
}

export default useCreateBlankAgent
