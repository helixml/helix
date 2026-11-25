import { createContext, FC, ReactNode, useCallback, useContext, useMemo, useRef, useState } from 'react'

/**
 * Lets an error inside a chat open the model picker in that chat's composer,
 * so "switch model" is one click away from the failure that calls for it.
 *
 * AgentChat provides it; the composer's CodeAgentConfigPicker registers its
 * opener. Pickers rendered elsewhere (task details, settings) sit outside the
 * provider and never register.
 */
interface ModelSwitcherContextValue {
  /** Opens the picker, or undefined when this chat has none. */
  openModelSwitcher?: () => void
  register: (open: () => void) => () => void
}

const ModelSwitcherContext = createContext<ModelSwitcherContextValue | null>(null)

export const ModelSwitcherProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const openerRef = useRef<(() => void) | null>(null)
  const [hasOpener, setHasOpener] = useState(false)

  const register = useCallback((open: () => void) => {
    openerRef.current = open
    setHasOpener(true)
    return () => {
      if (openerRef.current === open) {
        openerRef.current = null
        setHasOpener(false)
      }
    }
  }, [])

  const value = useMemo<ModelSwitcherContextValue>(() => ({
    openModelSwitcher: hasOpener ? () => openerRef.current?.() : undefined,
    register,
  }), [hasOpener, register])

  return <ModelSwitcherContext.Provider value={value}>{children}</ModelSwitcherContext.Provider>
}

export const useModelSwitcher = () => useContext(ModelSwitcherContext)
