export const TRANSCRIPT_TOPIC_PREFIX = 's-transcript-'

export const isTranscriptTopic = (topicID?: string): boolean =>
  topicID?.startsWith(TRANSCRIPT_TOPIC_PREFIX) ?? false
