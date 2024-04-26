import React, { FC, ReactNode, createContext, useState } from 'react'

export type ILayoutToolbarRenderer = () => ReactNode

export interface ILayoutContext {
  toolbarContent?: ReactNode,
  setToolbarContent: (node?: ReactNode) => void,
}

export const LayoutContext = createContext<ILayoutContext>({
  setToolbarContent: () => {}
})

export const useLayoutContext = (): ILayoutContext => {
  const [ toolbarContent, setToolbarContent ] = useState<ReactNode>()
  
  return {
    toolbarContent,
    setToolbarContent,
  }
}

export const LayoutContextProvider: FC = ({ children }) => {
  const value = useLayoutContext()
  return (
    <LayoutContext.Provider value={ value }>
      { children }
    </LayoutContext.Provider>
  )
}