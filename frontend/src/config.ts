import {
  IHelixModel,
  ICreateSessionConfig,
} from './types'

export const HELIX_TEXT_MODELS: IHelixModel[] = [{
  id: 'helix-3.5',
  title: 'Helix 3.5',
  description: 'Llama3-8B, fast and good for everyday tasks',
}, {
  id: 'helix-4',
  title: 'Helix 4',
  description: 'Llama3 70B, smarter but a bit slower',
}, {
  id: 'helix-code',
  title: 'Helix Code',
  description: 'CodeLlama 70B from Meta, better than GPT-4 at code',
}, {
  id: 'helix-json',
  title: 'Helix JSON',
  description: 'Nous Hermes 2 Pro 7B, for function calling & JSON output',
}]

export const HELIX_DEFAULT_TEXT_MODEL = 'helix-3.5'

export const DEFAULT_SESSION_CONFIG: ICreateSessionConfig = {
  activeToolIDs: [],
  finetuneEnabled: true,
  ragEnabled: false,
  ragDistanceFunction: 'cosine', 
  ragThreshold: 0.2,
  ragResultsCount: 3,
  ragChunkSize: 1024,
  ragChunkOverflow: 20,
}