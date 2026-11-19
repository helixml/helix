import { createContext, useContext, useEffect, useRef } from 'react'

/**
 * Counts the messages the user has sent from the chat composer. Action cards
 * in the thread (e.g. PR proposals) collapse when it changes, so a big pending
 * card doesn't keep pushing the agent's live activity out of view once the
 * user has moved on with the conversation.
 */
export const ActionCardCollapseContext = createContext(0)

/** Calls `collapse` each time the user sends a message after this mounted. */
export function useCollapseOnUserMessage(collapse: () => void) {
  const signal = useContext(ActionCardCollapseContext)
  const seen = useRef(signal)
  useEffect(() => {
    if (signal === seen.current) return
    seen.current = signal
    collapse()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signal])
}
