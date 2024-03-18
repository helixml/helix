import { useState, useCallback } from 'react'

export interface ISessionConfig {
  setFormData: (formData: FormData) => FormData,
  finetuneEnabled: boolean,
  setFinetuneEnabled: (value: boolean) => void,
  ragEnabled: boolean,
  setRagEnabled: (value: boolean) => void,
  ragDistanceFunction: 'l2' | 'inner_product' | 'cosine',
  setRagDistanceFunction: (value: 'l2' | 'inner_product' | 'cosine') => void,
  ragThreshold: number,
  setRagThreshold: (value: number) => void,
  ragResultsCount: number,
  setRagResultsCount: (value: number) => void,
  ragChunkSize: number,
  setRagChunkSize: (value: number) => void,
  ragChunkOverflow: number,
  setRagChunkOverflow: (value: number) => void,
}

export const useSessionConfig = () => {
  const [finetuneEnabled, setFinetuneEnabled] = useState(true)
  const [ragEnabled, setRagEnabled] = useState(false)
  const [ragDistanceFunction, setRagDistanceFunction] = useState<'l2' | 'inner_product' | 'cosine'>('cosine')
  const [ragThreshold, setRagThreshold] = useState(0.2)
  const [ragResultsCount, setRagResultsCount] = useState(3)
  const [ragChunkSize, setRagChunkSize] = useState(1024)
  const [ragChunkOverflow, setRagChunkOverflow] = useState(20)

  const setFormData = useCallback((formData: FormData) => {
    formData.set('text_finetune_enabled', finetuneEnabled ? 'yes' : 'no')
    formData.set('rag_enabled', ragEnabled ? 'yes' : 'no')
    formData.set('rag_distance_function', ragDistanceFunction)
    formData.set('rag_threshold', ragThreshold.toString())
    formData.set('rag_results_count', ragResultsCount.toString())
    formData.set('rag_chunk_size', ragChunkSize.toString())
    formData.set('rag_chunk_overflow', ragChunkOverflow.toString())
    
    return formData
  }, [
    finetuneEnabled,
    ragEnabled,
    ragDistanceFunction,
    ragThreshold,
    ragResultsCount,
    ragChunkSize,
    ragChunkOverflow,
  ])

  return {
    setFormData,
    finetuneEnabled, setFinetuneEnabled,
    ragEnabled, setRagEnabled,
    ragDistanceFunction, setRagDistanceFunction,
    ragThreshold, setRagThreshold,
    ragResultsCount, setRagResultsCount,
    ragChunkSize, setRagChunkSize,
    ragChunkOverflow, setRagChunkOverflow
  }
}

export default useSessionConfig