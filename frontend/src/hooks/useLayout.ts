import { useContext } from 'react'

import { LayoutContext } from '../contexts/layout'

export const useLayout = () => {
  const layout = useContext(LayoutContext)
  return layout
}

export default useLayout