import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { FC, ReactNode } from 'react'

/**
 * Action cards (e.g. PR proposals) collapse once the user sends a message, so a
 * big pending card doesn't keep pushing the agent's live activity out of view
 * after the user has moved on with the conversation.
 */
interface ActionCardCollapseScopeValue {
  /** Messages the user has sent from the composer in this scope. */
  userMessageCount: number
  userSentMessage: () => void
}

const ActionCardCollapseContext = createContext<ActionCardCollapseScopeValue | null>(null)

/**
 * Shares one message count between a composer and the cards around it. Nested
 * scopes defer to the outermost one, so a surface that renders cards beside
 * the chat (rather than inside it) can wrap both.
 */
export const ActionCardCollapseScope: FC<{ children: ReactNode }> = ({ children }) => {
  const parent = useContext(ActionCardCollapseContext)
  const [userMessageCount, setUserMessageCount] = useState(0)
  const value = useMemo(() => ({
    userMessageCount,
    userSentMessage: () => setUserMessageCount((count) => count + 1),
  }), [userMessageCount])
  if (parent) return <>{children}</>
  return <ActionCardCollapseContext.Provider value={value}>{children}</ActionCardCollapseContext.Provider>
}

/** For the composer: tell the cards in scope that the user sent a message. */
export function useNotifyUserMessage(): () => void {
  const scope = useContext(ActionCardCollapseContext)
  return scope?.userSentMessage ?? noop
}

/** Calls `collapse` each time the user sends a message after this mounted. */
export function useCollapseOnUserMessage(collapse: () => void) {
  const count = useContext(ActionCardCollapseContext)?.userMessageCount ?? 0
  const seen = useRef(count)
  useEffect(() => {
    if (count === seen.current) return
    seen.current = count
    collapse()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [count])
}

const noop = () => {}
