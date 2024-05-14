import {
  IHelixModel,
  ICreateSessionConfig,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  ISessionType,
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
}, {
  id: 'helix-small',
  title: 'Helix Small',
  description: 'Phi-3 Mini 3.8B, fast and memory efficient',
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

export const EXAMPLE_PROMPTS = {
  [SESSION_TYPE_TEXT]: [
    "Draft a weekly newsletter focusing on [a specific topic] tailored for a particular [company type], covering all necessary updates and insights",
    "Prepare a pitch for [presentation topic] aimed at potential investors, highlighting key benefits, projections, and strategic advantages",
    "Compose a email regarding project timeline adjustments to a client, explaining the reasons, impacts, and the revised timelines",
    "Develop a market analysis report on [industry/market segment], identifying key trends, challenges, and opportunities for growth",
    "Write an executive summary for a strategic plan focusing on [specific objective], including background, strategy, and expected outcomes",
    "Create a business proposal for [product/service] targeting [specific audience], outlining the value proposition, competitive advantage, and financial projections"
  ],
  [SESSION_TYPE_IMAGE]: [
    "Generate a beautiful photograph of a [color] rose garden, on a [weather condition] day, with [sky features], [additional elements], and a [sky color]",
    "Create an image of an interior design for a [adjective describing luxury] master bedroom, featuring [materials] furniture, [style keywords]",
    "Vaporwave style, [vehicle type], [setting], intricately detailed, [color palette], [resolution] resolution, photorealistic, [artistic adjectives]",
    "Design a corporate brochure cover for a [industry] firm, featuring [architectural style], clean lines, and the company's color scheme",
    "Produce an infographic illustrating the growth of [topic] over the last decade, using [color palette] and engaging visuals",
    "Visualize data on customer satisfaction ratings for [product/service], highlighting key strengths and areas for improvement"
  ]
}

export const PROMPT_LABELS = {
  [SESSION_TYPE_TEXT]: 'Chat with Helix...',
  [SESSION_TYPE_IMAGE]: 'Describe what you want to see in an image...',
}

export const COLORS = {
  [SESSION_TYPE_TEXT]: '#ffff00',
  [SESSION_TYPE_IMAGE]: '#3BF959',
  'GREEN_BUTTON': '#d5f4fa',
  'GREEN_BUTTON_HOVER': '#00d5ff',
  'AI_BADGE': '#f0beb0',
}

export const TOOLBAR_HEIGHT = 78