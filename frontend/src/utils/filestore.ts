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

export const FILE_EXT_MAP: Record<string, string> = {
  'jsonl': 'json',
}

export const getFileExtension = (filename: string) => {
  const parts = filename.split('.')
  const ext = parts[parts.length - 1]
  return FILE_EXT_MAP[ext] || ext
}

export const isImage = (filename: string) => {
  if(!filename) return false
  if(filename.match(/\.(jpg)|(png)|(jpeg)|(gif)$/i)) return true
  return false
}