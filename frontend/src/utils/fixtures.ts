import { ISession, IInteraction, SESSION_CREATOR_USER, SESSION_CREATOR_ASSISTANT, SESSION_TYPE_TEXT, SESSION_MODE_INFERENCE, INTERACTION_STATE_COMPLETE } from '../types'

const loremIpsum = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.`

export const generateFixtureInteraction = (index: number, isUser: boolean): IInteraction => {
  const baseMessage = isUser ? 'Tell me more about this topic' : loremIpsum
  return {
    id: `fixture-interaction-${index}`,
    created: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    updated: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    scheduled: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    completed: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    creator: isUser ? SESSION_CREATOR_USER : SESSION_CREATOR_ASSISTANT,
    mode: SESSION_MODE_INFERENCE,
    runner: 'fixture-runner',
    message: `${baseMessage} (Interaction ${index})`,
    display_message: `${baseMessage} (Interaction ${index})`,
    progress: 100,
    files: [],
    finished: true,
    metadata: {},
    state: INTERACTION_STATE_COMPLETE,
    status: 'completed',
    error: '',
    lora_dir: '',
    data_prep_chunks: {},
    data_prep_stage: '',
    data_prep_limited: false,
    data_prep_limit: 0,
  }
}

export const generateFixtureSession = (numInteractions: number = 300): ISession => {
  const interactions: IInteraction[] = []
  
  // Generate alternating user and assistant interactions
  for (let i = 0; i < numInteractions; i++) {
    interactions.push(generateFixtureInteraction(i, i % 2 === 0))
  }

  return {
    id: 'fixture-session',
    name: 'Fixture Test Session',
    created: new Date(Date.now() - (1000 * 60 * 60 * 24)).toISOString(),
    updated: new Date().toISOString(),
    parent_session: '',
    parent_app: '',
    config: {
      original_mode: SESSION_MODE_INFERENCE,
      origin: {
        type: 'user_created'
      },
      shared: false,
      avatar: '',
      priority: false,
      document_ids: {},
      document_group_id: '',
      manually_review_questions: false,
      system_prompt: '',
      helix_version: '1.0.0',
      eval_run_id: '',
      eval_user_score: '',
      eval_user_reason: '',
      eval_manual_score: '',
      eval_manual_reason: '',
      eval_automatic_score: '',
      eval_automatic_reason: '',
      eval_original_user_prompts: [],
      rag_source_data_entity_id: '',
    },
    mode: SESSION_MODE_INFERENCE,
    type: SESSION_TYPE_TEXT,
    model_name: 'fixture-model',
    lora_dir: '',
    interactions,
    owner: 'fixture-owner',
    owner_type: 'user',
  }
} 