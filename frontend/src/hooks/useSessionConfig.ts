import { useState, useCallback, SetStateAction, Dispatch } from 'react'

export type IRagDistanceFunction = 'l2' | 'inner_product' | 'cosine'
export interface ISessionConfig {
  setFormData: (formData: FormData) => FormData,
  activeToolIDs: string[],
  setActiveToolIDs: Dispatch<SetStateAction<string[]>>,
  finetuneEnabled: boolean,
  setFinetuneEnabled: Dispatch<SetStateAction<boolean>>,
  ragEnabled: boolean,
  setRagEnabled: Dispatch<SetStateAction<boolean>>,
  ragDistanceFunction: IRagDistanceFunction,
  setRagDistanceFunction: Dispatch<SetStateAction<IRagDistanceFunction>>,
  ragThreshold: number,
  setRagThreshold: Dispatch<SetStateAction<number>>,
  ragResultsCount: number,
  setRagResultsCount: Dispatch<SetStateAction<number>>,
  ragChunkSize: number,
  setRagChunkSize: Dispatch<SetStateAction<number>>,
  ragChunkOverflow: number,
  setRagChunkOverflow: Dispatch<SetStateAction<number>>,
}

export const useSessionConfig = () => {
  const [finetuneEnabled, setFinetuneEnabled] = useState(true)
  const [activeToolIDs, setActiveToolIDs] = useState<string[]>([])
  const [ragEnabled, setRagEnabled] = useState(false)
  const [ragDistanceFunction, setRagDistanceFunction] = useState<'l2' | 'inner_product' | 'cosine'>('cosine')
  const [ragThreshold, setRagThreshold] = useState(0.2)
  const [ragResultsCount, setRagResultsCount] = useState(3)
  const [ragChunkSize, setRagChunkSize] = useState(1024)
  const [ragChunkOverflow, setRagChunkOverflow] = useState(20)

  const setFormData = useCallback((formData: FormData) => {
    formData.set('active_tools', activeToolIDs.join(','))
    formData.set('text_finetune_enabled', finetuneEnabled ? 'yes' : '')
    formData.set('rag_enabled', ragEnabled ? 'yes' : '')
    formData.set('rag_distance_function', ragDistanceFunction)
    formData.set('rag_threshold', ragThreshold.toString())
    formData.set('rag_results_count', ragResultsCount.toString())
    formData.set('rag_chunk_size', ragChunkSize.toString())
    formData.set('rag_chunk_overflow', ragChunkOverflow.toString())
    return formData
  }, [
    activeToolIDs,
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
    activeToolIDs, setActiveToolIDs,
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