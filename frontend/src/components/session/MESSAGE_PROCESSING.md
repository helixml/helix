# Message Processing Architecture

This document explains the centralized message processing architecture used in the Helix frontend.

## Overview

Message processing (handling citations, document links, thinking tags, and blinkers) is centralized in the `Markdown.tsx` component using a `MessageProcessor` class. This architecture eliminates duplicated logic that was previously spread across multiple components.

## Flow

1. The raw message text from the LLM is passed directly to the `Markdown` component from either:
   - `InteractionLiveStream` (for live, streaming responses)
   - `InteractionInference` (for completed responses)

2. The `Markdown` component processes the text using the `MessageProcessor` with the following steps:
   - Extract citation blocks for special handling
   - Process document ID links
   - Process document group ID links
   - Add blinker if needed (for streaming responses)
   - Sanitize HTML (escape non-standard tags)
   - Process thinking tags
   - Restore preserved content

3. The processed message is then rendered using React Markdown

## Benefits

- **Single Source of Truth**: All message processing happens in one place
- **Simplified Components**: LiveStream and Inference components don't need to do preprocessing
- **Consistent Formatting**: All messages are processed the same way
- **Easier Maintenance**: Processing logic changes only need to be made in one place

## Key Components

### MessageProcessor

The `MessageProcessor` class handles all text processing in a structured way:

```typescript
class MessageProcessor {
  // Properties
  private session: ISession;
  private getFileURL: (filename: string) => string;
  private showBlinker: boolean;
  private isStreaming: boolean;
  
  // Processing steps
  process(): string {
    this.extractCitations();
    this.processDocumentIDLinks();
    this.processGroupIDLinks();
    this.addBlinkerIfNeeded();
    this.sanitizeHTML();
    this.processThinkingTags();
    this.restorePreservedContent();
    
    return this.resultContent;
  }
  
  // ... other methods handling specific parts of processing
}
```

### InteractionMarkdown Component

The `InteractionMarkdown` component accepts these props:

```typescript
export interface InteractionMarkdownProps {
  text: string;               // Raw message text from LLM
  session?: ISession;         // Session object with document IDs and metadata
  getFileURL?: Function;      // Function to generate file URLs
  showBlinker?: boolean;      // Whether to show the blinking cursor
  isStreaming?: boolean;      // Whether this is a streaming message
}
```

## Usage

To render a processed message:

```jsx
<Markdown
  text={message}                // Raw message from LLM
  session={session}             // Session object
  getFileURL={getFileURL}       // Function to generate file URLs
  showBlinker={true}            // For streaming messages
  isStreaming={true}            // For streaming context
/>
``` 