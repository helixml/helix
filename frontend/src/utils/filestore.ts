import {
  IFileStoreItem,
  IFileStoreFolder,
  IFileStoreConfig,
} from '../types'

export const getRelativePath = (config: IFileStoreConfig, file: IFileStoreItem) =>{
  const { user_prefix } = config
  const { path } = file
  if (path.startsWith(user_prefix)) {
    return path.substring(user_prefix.length)
  }
  return path
}

export const isPathReadonly = (config: IFileStoreConfig, path: string) =>{
  const parts = path.split('/')
  const rootFolder = parts[1]
  if(!rootFolder) return false
  const folder = config.folders.find(f => f.name === rootFolder)
  return folder?.readonly || false
}