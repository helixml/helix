import { useState, useCallback } from 'react'
import bluebird from 'bluebird'
import { AxiosProgressEvent } from 'axios'

import {
  ISerializedPage,
} from '../types'

import {
  IFilestoreUploadProgress,
} from '../contexts/filestore'

import {
  serializeFile,
  deserializeFile,
  saveFile,
  loadFile,
  deleteFile,
} from '../utils/filestore'

export interface IFinetuneInputs {
  inputValue: string,
  setInputValue: (value: string) => void,
  manualTextFileCounter: number,
  setManualTextFileCounter: (value: number) => void,
  fineTuneStep: number,
  setFineTuneStep: (value: number) => void,
  showImageLabelErrors: boolean,
  setShowImageLabelErrors: (value: boolean) => void,
  files: File[],
  setFiles: (value: File[]) => void,
  labels: Record<string, string>,
  setLabels: (value: Record<string, string>) => void,
  uploadProgress: IFilestoreUploadProgress | undefined,
  setUploadProgress: (value: IFilestoreUploadProgress | undefined) => void,
  serializePage: () => Promise<void>,
  loadFromLocalStorage: () => Promise<void>,
  setFormData: (formData: FormData) => FormData,
  uploadProgressHandler: (progressEvent: AxiosProgressEvent) => void,
  reset: () => Promise<void>,
}

export const useFinetuneInputs = () => {
  const [inputValue, setInputValue] = useState('')
  const [manualTextFileCounter, setManualTextFileCounter] = useState(0)
  const [uploadProgress, setUploadProgress] = useState<IFilestoreUploadProgress>()
  const [fineTuneStep, setFineTuneStep] = useState(0)
  const [showImageLabelErrors, setShowImageLabelErrors] = useState(false)
  const [files, setFiles] = useState<File[]>([])
  const [labels, setLabels] = useState<Record<string, string>>({})
  
  const serializePage = useCallback(async () => {
    const serializedFiles = await bluebird.map(files, async (file) => {
      const serializedFile = await serializeFile(file)
      await saveFile(serializedFile)
      serializedFile.content = ''
      return serializedFile
    })
    const data: ISerializedPage = {
      files: serializedFiles,
      labels,
      fineTuneStep,
      manualTextFileCounter,
      inputValue,
    }
    localStorage.setItem('new-page', JSON.stringify(data))
  }, [
    files,
    labels,
    fineTuneStep,
    manualTextFileCounter,
    inputValue,
  ])

  const setFormData = useCallback((formData: FormData) => {
    files.forEach((file) => {
      formData.append("files", file)
      if(labels[file.name]) {
        formData.set(file.name, labels[file.name])
      }
    })
    return formData
  }, [
    files,
    labels,
  ])

  const uploadProgressHandler = useCallback((progressEvent: AxiosProgressEvent) => {
    const percent = progressEvent.total && progressEvent.total > 0 ?
      Math.round((progressEvent.loaded * 100) / progressEvent.total) :
      0
    setUploadProgress({
      percent,
      totalBytes: progressEvent.total || 0,
      uploadedBytes: progressEvent.loaded || 0,
    })
  }, [])

  const loadFromLocalStorage = useCallback(async () => {
    const dataString = localStorage.getItem('new-page')
    if(!dataString) {
      return
    }
    localStorage.removeItem('new-page')
    const data: ISerializedPage = JSON.parse(dataString)
    // map over the empty content files
    // load their content from the individual file key
    // turn into native File
    const loadedFiles = await bluebird.map(data.files, async file => {
      const loadedFile = await loadFile(file)
      await deleteFile(file)
      return deserializeFile(loadedFile)
    })
    setFiles(loadedFiles)
    setLabels(data.labels)
    setFineTuneStep(data.fineTuneStep)
    setManualTextFileCounter(data.manualTextFileCounter)
    setInputValue(data.inputValue)
  }, [])

  const reset = useCallback(async () => {
    setFiles([])
    setLabels({})
    setFineTuneStep(0)
    setManualTextFileCounter(0)
    setInputValue('')
  }, [])
  
  return {
    inputValue, setInputValue,
    manualTextFileCounter, setManualTextFileCounter,
    fineTuneStep, setFineTuneStep,
    showImageLabelErrors, setShowImageLabelErrors,
    files, setFiles,
    labels, setLabels,
    uploadProgress, setUploadProgress,
    serializePage,
    loadFromLocalStorage,
    setFormData,
    uploadProgressHandler,
    reset,
  }
}

export default useFinetuneInputs