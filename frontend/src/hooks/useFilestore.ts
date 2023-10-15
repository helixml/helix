import { useContext } from 'react'

import {
  FilestoreContext,
} from '../contexts/filestore'

export const useFilestore = () => {
  const filestore = useContext(FilestoreContext)
  return filestore
}

export default useFilestore