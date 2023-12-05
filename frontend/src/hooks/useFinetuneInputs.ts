import { useState, useCallback } from 'react'
import bluebird from 'bluebird'
import {AxiosProgressEvent} from 'axios'

import useApi from './useApi'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
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

  const getFormData = useCallback(async (mode: string, type: string) => {
    const formData = new FormData()
      files.forEach((file) => {
        formData.append("files", file)
        if(labels[file.name]) {
          formData.set(file.name, labels[file.name])
        }
      })
      formData.set('mode', mode)
      formData.set('type', type)
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
    getFormData,
    uploadProgressHandler,
  }
}

export default useFinetuneInputs