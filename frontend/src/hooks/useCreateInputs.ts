import { useState, useCallback, SetStateAction, Dispatch } from 'react'
import bluebird from 'bluebird'
import { AxiosProgressEvent } from 'axios'

import {
  ISessionMode,
  ISessionType,
  IUploadFile,
  ISerializedPage,
  ICreateSessionConfig,
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

import {
  DEFAULT_SESSION_CONFIG,
} from '../config'

export interface IFinetuneInputs {
  inputValue: string,
  setInputValue: Dispatch<SetStateAction<string>>,
  manualTextFileCounter: number,
  setManualTextFileCounter: Dispatch<SetStateAction<number>>,
  fineTuneStep: number,
  setFineTuneStep: Dispatch<SetStateAction<number>>,
  sessionConfig: ICreateSessionConfig,
  setSessionConfig: Dispatch<SetStateAction<ICreateSessionConfig>>,
  showImageLabelErrors: boolean,
  setShowImageLabelErrors: Dispatch<SetStateAction<boolean>>,
  finetuneFiles: IUploadFile[],
  setFinetuneFiles: Dispatch<SetStateAction<IUploadFile[]>>,
  labels: Record<string, string>,
  setLabels: Dispatch<SetStateAction<Record<string, string>>>,
  uploadProgress: IFilestoreUploadProgress | undefined,
  setUploadProgress: Dispatch<SetStateAction<IFilestoreUploadProgress | undefined>>,
  serializePage: () => Promise<void>,
  loadFromLocalStorage: () => Promise<void>,
  setFormData: (formData: FormData) => FormData,
  uploadProgressHandler: (progressEvent: AxiosProgressEvent) => void,
  getFormData: (mode: ISessionMode, type: ISessionType, model: string) => FormData,
  getUploadedFiles: () => FormData,
  reset: () => Promise<void>,
}

export const useCreateInputs = () => {
  const [inputValue, setInputValue] = useState('')
  const [sessionConfig, setSessionConfig] = useState<ICreateSessionConfig>(DEFAULT_SESSION_CONFIG)
  const [manualTextFileCounter, setManualTextFileCounter] = useState(0)
  const [uploadProgress, setUploadProgress] = useState<IFilestoreUploadProgress>()
  const [fineTuneStep, setFineTuneStep] = useState(0)
  const [showImageLabelErrors, setShowImageLabelErrors] = useState(false)
  const [finetuneFiles, setFinetuneFiles] = useState<IUploadFile[]>([])
  const [labels, setLabels] = useState<Record<string, string>>({})

  const serializePage = useCallback(async () => {
    const drawerLabels: Record<string, string> = {}
    const serializedFiles = await bluebird.map(finetuneFiles, async (file) => {
      drawerLabels[file.file.name] = file.drawerLabel
      const serializedFile = await serializeFile(file.file)
      await saveFile(serializedFile)
      serializedFile.content = ''
      return serializedFile
    })
    const data: ISerializedPage = {
      files: serializedFiles,
      drawerLabels,
      labels,
      fineTuneStep,
      manualTextFileCounter,
      inputValue,
    }
    localStorage.setItem('new-page', JSON.stringify(data))
  }, [
    finetuneFiles,
    labels,
    fineTuneStep,
    manualTextFileCounter,
    inputValue,
  ])

  const getFormData = useCallback((mode: ISessionMode, type: ISessionType, model: string): FormData => {
    const formData = new FormData()

    formData.set('input', inputValue)
    formData.set('mode', mode)
    formData.set('type', type)
    formData.set('helixModel', model)

    formData.set('active_tools', sessionConfig.activeToolIDs.join(','))
    formData.set('text_finetune_enabled', sessionConfig.finetuneEnabled ? 'yes' : '')
    formData.set('rag_enabled', sessionConfig.ragEnabled ? 'yes' : '')
    formData.set('rag_distance_function', sessionConfig.ragDistanceFunction)
    formData.set('rag_threshold', sessionConfig.ragThreshold.toString())
    formData.set('rag_results_count', sessionConfig.ragResultsCount.toString())
    formData.set('rag_chunk_size', sessionConfig.ragChunkSize.toString())
    formData.set('rag_chunk_overflow', sessionConfig.ragChunkOverflow.toString())

    finetuneFiles.forEach((file) => {
      formData.append("files", file.file)
      if(labels[file.file.name]) {
        formData.set(file.file.name, labels[file.file.name])
      }
    })

    return formData
  }, [
    inputValue,
    finetuneFiles,
    labels,
    sessionConfig,
  ])

  const getUploadedFiles = useCallback((): FormData => {
    const formData = new FormData()
    finetuneFiles.forEach((file) => {
      formData.append("files", file.file)
      if(labels[file.file.name]) {
        formData.set(file.file.name, labels[file.file.name])
      }
    })
    return formData
  }, [
    finetuneFiles,
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
      const deserializedFile = deserializeFile(loadedFile)
      const uploadedFile: IUploadFile = {
        drawerLabel: data.drawerLabels[deserializedFile.name],
        file: deserializedFile,
      }
      return uploadedFile
    })
    setFinetuneFiles(loadedFiles)
    setLabels(data.labels)
    setFineTuneStep(data.fineTuneStep)
    setManualTextFileCounter(data.manualTextFileCounter)
    setInputValue(data.inputValue)
  }, [])

  const reset = useCallback(async () => {
    setFinetuneFiles([])
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
    sessionConfig, setSessionConfig,
    finetuneFiles, setFinetuneFiles,
    labels, setLabels,
    uploadProgress, setUploadProgress,
    serializePage,
    loadFromLocalStorage,
    getFormData,
    getUploadedFiles,
    uploadProgressHandler,
    reset,
  }
}

export default useCreateInputs