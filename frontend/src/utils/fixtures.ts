import { TypesMessageContent, TypesMessageContentType, TypesInteraction, TypesSessionMode, TypesInteractionState, TypesSession, TypesSessionType } from '../api/api'

const loremIpsum = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.`

export const generateFixtureInteraction = (index: number, isUser: boolean): TypesInteraction => {
  const baseMessage = isUser ? 'Tell me more about this topic' : loremIpsum
  return {
    id: `fixture-interaction-${index}`,
    created: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    updated: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    scheduled: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    completed: new Date(Date.now() - (1000 * 60 * 60 * (300 - index))).toISOString(),
    mode: TypesSessionMode.SessionModeInference,
    runner: 'fixture-runner',
    prompt_message: `${baseMessage} (Interaction ${index})`,
    prompt_message_content: {
      content_type: TypesMessageContentType.MessageContentTypeText,
      parts: [`${baseMessage} (Interaction ${index})`]
    } as TypesMessageContent,
    display_message: `${baseMessage} (Interaction ${index})`,    
    state: TypesInteractionState.InteractionStateComplete,
    status: 'completed',
    error: '',    
  }
}

export const generateFixtureSession = (numInteractions: number = 300): TypesSession => {
  const interactions: TypesInteraction[] = []

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
      avatar: '',
      priority: false,
      document_ids: {},
      document_group_id: '',
      session_rag_results: [],
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
    },
    mode: TypesSessionMode.SessionModeInference,
    type: TypesSessionType.SessionTypeText,
    provider: 'fixture-provider',
    model_name: 'fixture-model',
    lora_dir: '',
    interactions,
    owner: 'fixture-owner',
  }
} 