import React, { FC, ReactNode, createContext, useState } from 'react'

export type ILayoutToolbarRenderer = (bigScreen: boolean) => ReactNode

export interface ILayoutContext {
  toolbarRenderer?: ILayoutToolbarRenderer,
  setToolbarRenderer: (renderer?: ILayoutToolbarRenderer) => void,
}

export const LayoutContext = createContext<ILayoutContext>({
  setToolbarRenderer: () => {}
})

export const useLayoutContext = (): ILayoutContext => {
  const [ toolbarRenderer, setToolbarRenderer ] = useState<ILayoutToolbarRenderer>()
  
  return {
    toolbarRenderer,
    setToolbarRenderer,
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