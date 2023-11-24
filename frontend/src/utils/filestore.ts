import ldb from 'localdata'
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

export interface ISerlializedFile {
  filename: string
  content: string
  mimeType: string
}

// return a JSON representation of the file
// with base64 contents
export const serializeFile = async (file: File): Promise<ISerlializedFile> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.readAsDataURL(file)
    reader.onload = () => {
      const base64Content = reader.result?.toString().split(',')[1] || ''
      const mimeType = file.type || ''
      const serializedFile: ISerlializedFile = {
        filename: file.name,
        content: base64Content,
        mimeType: mimeType
      }
      return resolve(serializedFile)
    }
    reader.onerror = error => reject(error)
  })
}

function base64ToFile(base64String: string, filename: string, mimeType: string): File {
  // Decode the base64 string to a binary string
  const binaryString = window.atob(base64String);

  // Convert binary string to a Uint8Array
  const binaryLen = binaryString.length;
  const bytes = new Uint8Array(binaryLen);
  for (let i = 0; i < binaryLen; i++) {
      bytes[i] = binaryString.charCodeAt(i);
  }

  // Create a blob from the Uint8Array
  const blob = new Blob([bytes], { type: mimeType });

  // Create a File object from the Blob
  const file = new File([blob], filename, { type: mimeType });
  return file;
}

export const deserializeFile = (data: ISerlializedFile): File => {
  return base64ToFile(data.content, data.filename, data.mimeType)
}

export const saveFile = async (file: ISerlializedFile): Promise<void> => {
  return new Promise((resolve, reject) => {
    ldb.set(file.filename, file.content, () => {
      resolve()
    })
  })
}

// we have saved a file with empty content in the main doc
// the file contents are saved in seperate files so we can save more files
export const loadFile = async (file: ISerlializedFile): Promise<ISerlializedFile> => {
  const fileContent = await new Promise((resolve, reject) => {
    ldb.get(file.filename, (value) => {
      resolve(value)
    })
  })
  return {
    ...file,
    content: fileContent as string,
  }
}

export const deleteFile = async (file: ISerlializedFile): Promise<void> => {
  return new Promise((resolve, reject) => {
    ldb.delete(file.filename, () => {
      resolve()
    })
  })
}