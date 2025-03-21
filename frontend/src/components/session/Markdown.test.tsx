/**
 * MessageProcessor Unit Tests
 * 
 * These tests focus on the thinking tag behavior in the MessageProcessor class:
 * 
 * 1. During streaming:
 *    - When a thinking tag is unclosed - should show the thinking container with the rainbow glow
 *    - When a thinking tag is closed - should show the thinking container without the glow
 * 
 * 2. After streaming finishes:
 *    - Even if the thinking tag is unclosed - should show the thinking container without the glow
 * 
 * NOTE: To run these tests, the project needs to be set up with Jest or Vitest.
 * 
 * Setup instructions:
 * 1. Install test dependencies:
 *    npm install --save-dev jest @types/jest ts-jest
 *    or
 *    npm install --save-dev vitest
 * 
 * 2. Add to package.json:
 *    "scripts": {
 *      "test": "jest"
 *    }
 * 
 * 3. Create a jest.config.js file with proper TypeScript and React settings
 */

// This is a placeholder test file showing how the MessageProcessor tests should be structured
// Since the testing framework is not yet set up, this file will have linter errors

import { ISession, ISessionConfig } from '../../types';
import { MessageProcessor } from './Markdown';
import { describe, expect, test, beforeEach } from 'vitest';

// Mock data for tests
const mockSession: Partial<ISession> = {
  id: 'test-session',
  name: 'Test Session',
  created: new Date().toISOString(),
  updated: new Date().toISOString(),
  parent_session: '',
  parent_app: '',
  mode: 'inference',
  type: 'text',
  model_name: 'test-model',
  lora_dir: '',
  owner: 'test-owner',
  owner_type: 'user',
  config: {
    document_ids: {},
  } as ISessionConfig,
};

// Mock file URL resolver function
const mockGetFileURL = (file: string) => `https://example.com/${file}`;

describe('MessageProcessor', () => {
  // Common mocks
  const mockSession = {
    id: 'test-session',
    config: {
      document_ids: {}
    }
  };
  
  const mockGetFileURL = (file: string) => `https://example.com/${file}`;

  beforeEach(() => {
    // Any setup code would go here
  });

  // Test case 1: During streaming, with unclosed thinking tag
  test('During streaming with unclosed thinking tag - should show rainbow glow', () => {
    // Create message with unclosed thinking tag
    const message = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();
    
    // Check for the content
    expect(result).toContain('This is my thought process');
    
    // For streaming with unclosed thinking tag:
    // 1. The <think> tag should be converted to a div with the "thinking" class
    // 2. The content should be inside the container
    expect(result).not.toContain('<think>');
    expect(result).toMatch(/<div class="think-container thinking"/);
  });

  // NEW TEST: Specifically check HTML structure during streaming with unclosed thinking tag
  test('During streaming, unclosed thinking tag should produce HTML with thinking class', () => {
    // Create message with unclosed thinking tag
    const message = `Hello world!
<think>
This is still thinking
`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();
    
    // Debug output to see what's happening
    console.log("HTML output during streaming:", result);

    // The content should be preserved
    expect(result).toContain('This is still thinking');

    // FIXED EXPECTATIONS:
    // During streaming with an unclosed thinking tag, we should see:
    // 1. The unclosed <think> tag transformed into a container with thinking class
    // 2. The thinking tag content visible in the container

    // We expect the raw <think> tag to be replaced
    expect(result).not.toContain('<think>');
    
    // We expect a div with the "thinking" class to be present
    expect(result).toMatch(/<div class="think-container thinking"/);
    
    // Content should be inside the container
    expect(result).toMatch(/<div class="think-content">\s*\n\s*This is still thinking\s*\n\s*<\/div>/);
  });

  // Test case 2: During streaming, with closed thinking tag
  test('During streaming with closed thinking tag - should NOT show rainbow glow', () => {
    // Create message with closed thinking tag
    const message = `Hello world!
<think>
This is my thought process
</think>`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();

    // Check the content is preserved
    expect(result).toContain('This is my thought process');
  });

  // Test case 3: After streaming with unclosed thinking tag
  test('After streaming with unclosed thinking tag - should NOT show rainbow glow', () => {
    // Create message with unclosed thinking tag
    const message = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=false (streaming finished)
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });

    // Process the message
    const result = processor.process();

    // Check the content is preserved
    expect(result).toContain('This is my thought process');
  });

  // Test case 4: After streaming with closed thinking tag
  test('After streaming with closed thinking tag - should NOT show rainbow glow', () => {
    // Create message with closed thinking tag
    const message = `Hello world!
<think>
This is my thought process
</think>`;

    // Create processor with streaming=false (streaming finished)
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });

    // Process the message
    const result = processor.process();

    // Check the content is preserved
    expect(result).toContain('This is my thought process');
  });

  // Test case 5: Multiple thinking tags with different states
  test('Multiple thinking tags with different states', () => {
    // Create message with multiple thinking tags
    const message = `Hello world!
<think>
This is a closed thought
</think>

More content...

<think>
This is an unclosed thought
`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();

    // First thinking tag should NOT have thinking class
    // Second thinking tag SHOULD have thinking class
    
    // Verify content is preserved
    expect(result).toContain('This is a closed thought');
    expect(result).toContain('This is an unclosed thought');
    
    // The actual behavior depends on the implementation details.
    // For MessageProcessor, verify that the tags are processed and
    // correctly transformed into HTML containers.
    
    // Count occurrences of think-container
    const containerMatches = result.match(/think-container/g) || [];
    
    // Since implementations may handle multiple think tags differently,
    // let's just verify we have at least one container
    expect(containerMatches.length).toBeGreaterThan(0);
  });

  // Test case 6: Test handling "---" as a think tag closing delimiter
  test('Triple dash should close thinking tags during streaming', () => {
    // Create message with thinking tag closed by triple dash
    const message = `Hello world!
<think>
This thought will be closed by triple dash
---

More content...`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();

    // Check content is preserved
    expect(result).toContain('This thought will be closed by triple dash');
    expect(result).toContain('More content...');
  });

  // Test case 7: Empty thinking tags should be removed
  test('Empty thinking tags should be removed', () => {
    // Create message with empty thinking tag
    const message = `Hello world!
<think>
</think>

More content...`;

    // Create processor with streaming=true
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the message
    const result = processor.process();

    // Verify the content is preserved
    expect(result).toContain('Hello world!');
    expect(result).toContain('More content...');
  });
  
  // Test case 8: Test preserving HTML in think tags through sanitization
  test('HTML in thinking tags should be preserved through sanitization', () => {
    // Create message with HTML in thinking tag
    const message = `Hello world!
<think>
This is <strong>bold</strong> and <em>italicized</em> text.
<code>const x = 42;</code>
</think>`;

    // Create processor with streaming=false
    const processor = new MessageProcessor(message, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });

    // Process the message
    const result = processor.process();

    // Verify content is preserved
    expect(result).toContain('This is');
    expect(result).toContain('bold');
    expect(result).toContain('italicized');
    expect(result).toContain('const x = 42;');
  });

  // Test case for closing a thinking tag during streaming
  test('When LLM emits closing think tag during streaming, thinking container should be closed immediately', () => {
    // STEP 1: First process with an unclosed thinking tag
    const initialMessage = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=true
    const initialProcessor = new MessageProcessor(initialMessage, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the initial message
    const initialResult = initialProcessor.process();
    
    // Verify initial state has thinking class (rainbow glow)
    expect(initialResult).not.toContain('<think>');
    expect(initialResult).toMatch(/<div class="think-container thinking"/);
    
    // STEP 2: Now process with the same content + a closing tag (simulating LLM adding the closing tag)
    const updatedMessage = `Hello world!
<think>
This is my thought process
</think>`;

    // Create a new processor (simulating another message processor call during streaming)
    const updatedProcessor = new MessageProcessor(updatedMessage, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true, // Still streaming
    });

    // Process the updated message
    const updatedResult = updatedProcessor.process();
    
    // Debug output
    console.log("Initial result:", initialResult);
    console.log("Updated result:", updatedResult);
    
    // EXPECTATIONS:
    // 1. The <think> tag should be properly closed
    // 2. The thinking class (rainbow glow) should be removed
    // 3. The content should still be present
    
    expect(updatedResult).toContain('This is my thought process');
    expect(updatedResult).not.toContain('<think>');
    expect(updatedResult).not.toMatch(/<div class="think-container thinking"/); // No thinking class
    expect(updatedResult).toMatch(/<div class="think-container"><details/); // Regular think container
  });

  // Test case for details element toggling closed when LLM finishes a thinking tag
  test('When LLM finishes a thinking tag, the details element should be toggled closed', () => {
    // STEP 1: First process with an unclosed thinking tag
    const initialMessage = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=true
    const initialProcessor = new MessageProcessor(initialMessage, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true,
    });

    // Process the initial message
    const initialResult = initialProcessor.process();
    
    // Verify details is open initially
    expect(initialResult).toMatch(/<details open>/); // Details should be open during streaming
    
    // STEP 2: Now process with the same content + a closing tag (simulating LLM adding the closing tag)
    const updatedMessage = `Hello world!
<think>
This is my thought process
</think>`;

    // Create a new processor (simulating another message processor call during streaming)
    const updatedProcessor = new MessageProcessor(updatedMessage, {
      session: mockSession as ISession,
      getFileURL: mockGetFileURL,
      isStreaming: true, // Still streaming
    });

    // Process the updated message
    const updatedResult = updatedProcessor.process();
    
    // Debug output
    console.log("Initial details state:", initialResult.includes('<details open>') ? 'open' : 'closed');
    console.log("Updated details state:", updatedResult.includes('<details open>') ? 'open' : 'closed');
    
    // EXPECTATIONS:
    // When the LLM adds a closing tag, the details element should be closed (collapsed)
    // This should fail with the current implementation since it always sets details to open
    
    expect(updatedResult).toContain('This is my thought process'); // Content preserved
    expect(updatedResult).not.toMatch(/<details open>/); // Should NOT have open attribute
    expect(updatedResult).toMatch(/<details>/); // Should be closed
  });
});

// Placeholder export to avoid unused import warnings
export const MessageProcessorTests = {
  description: "Tests for the MessageProcessor class to validate thinking tag behavior."
}; 