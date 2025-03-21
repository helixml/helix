import { describe, it, expect } from 'vitest';

// This is a simplified version of the thinking tag processor logic 
// from the MessageProcessor class
class ThinkTagProcessor {
  private message: string;
  private isStreaming: boolean;

  constructor(message: string, isStreaming: boolean) {
    this.message = message;
    this.isStreaming = isStreaming;
  }

  processThinkingTags(): string {
    // Process thinking tags with the same logic as in MessageProcessor
    let result = this.message;
    let openCount = 0;
    
    result = result.split('\n').map(line => {
      if (line.includes('<think>')) openCount++;
      if (line.includes('</think>')) openCount--;
      if (line.trim() === '---' && openCount > 0) {
        openCount--;
        return '</think>';
      }
      return line;
    }).join('\n');

    // Check if there's an unclosed think tag
    const openTagCount = (result.match(/<think>/g) || []).length;
    const closeTagCount = (result.match(/<\/think>/g) || []).length;
    const isThinking = openTagCount > closeTagCount;
    
    // Create a working copy we can modify
    let processedResult = result;
    
    // Special handling for unclosed tags during streaming
    if (isThinking) {
      if (!this.isStreaming) {
        // Auto-close tag if we're not streaming
        processedResult += '\n</think>';
      } else {
        // For streaming with unclosed tags, we need to handle them specially
        // Add a temporary closing tag that will be removed after processing
        processedResult += '\n</think>__TEMP_CLOSING_TAG__';
      }
    }

    // Show the rainbow glow ONLY when:
    // 1. We are streaming AND 
    // 2. There's an unclosed thinking tag
    const showGlow = this.isStreaming && isThinking;
    const thinkingClass = showGlow ? ' thinking' : '';
    
    // Convert <think> tags to styled divs
    processedResult = processedResult.replace(
      /<think>([\s\S]*?)<\/think>(?:__TEMP_CLOSING_TAG__)?/g,
      (match, content) => {
        const trimmedContent = content.trim();
        if (!trimmedContent) return ''; // Skip empty think tags

        // If this is a temporary closed tag during streaming and has unclosed rainbow glow
        const hasTemporaryClosing = match.includes('__TEMP_CLOSING_TAG__');
        const containerClass = hasTemporaryClosing && showGlow ? 
          'think-container thinking' : 'think-container';
          
        return `<div class="${containerClass}"><details open><summary class="think-header"><strong>Reasoning</strong></summary><div class="think-content">

${trimmedContent}

</div></details></div>`;
      }
    );

    return processedResult;
  }
}

describe('ThinkTagProcessor', () => {
  // Test case 1: During streaming, with unclosed thinking tag
  it('During streaming with unclosed thinking tag - should show rainbow glow', () => {
    // Create message with unclosed thinking tag
    const message = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=true
    const processor = new ThinkTagProcessor(message, true);
    
    // Process the message
    const result = processor.processThinkingTags();

    // Assert that result contains think-container with thinking class (for rainbow glow)
    expect(result).toContain('think-container thinking');
    expect(result).toContain('This is my thought process');
  });

  // Test case 2: During streaming, with closed thinking tag
  it('During streaming with closed thinking tag - should NOT show rainbow glow', () => {
    // Create message with closed thinking tag
    const message = `Hello world!
<think>
This is my thought process
</think>`;

    // Create processor with streaming=true
    const processor = new ThinkTagProcessor(message, true);
    
    // Process the message
    const result = processor.processThinkingTags();

    // Assert that result contains think-container WITHOUT thinking class
    expect(result).toContain('think-container');
    expect(result).not.toContain('think-container thinking');
    expect(result).toContain('This is my thought process');
  });

  // Test case 3: After streaming finished, with unclosed thinking tag
  it('After streaming with unclosed thinking tag - should NOT show rainbow glow', () => {
    // Create message with unclosed thinking tag
    const message = `Hello world!
<think>
This is my thought process
`;

    // Create processor with streaming=false (streaming finished)
    const processor = new ThinkTagProcessor(message, false);
    
    // Process the message
    const result = processor.processThinkingTags();

    // Assert that result contains think-container WITHOUT thinking class
    expect(result).toContain('think-container');
    expect(result).not.toContain('think-container thinking');
    expect(result).toContain('This is my thought process');
    
    // Should have auto-closed the thinking tag
    expect(result).toContain('</div></details></div>');
  });

  // Test case 4: Triple dash should close thinking tags during streaming
  it('Triple dash should close thinking tags during streaming', () => {
    // Create message with thinking tag closed by triple dash
    const message = `Hello world!
<think>
This thought will be closed by triple dash
---

More content...`;

    // Create processor with streaming=true
    const processor = new ThinkTagProcessor(message, true);
    
    // Process the message
    const result = processor.processThinkingTags();

    // The thought should be closed and NOT have thinking class
    expect(result).toContain('think-container');
    expect(result).not.toContain('think-container thinking');
    expect(result).toContain('This thought will be closed by triple dash');
  });

  // Test case 5: Empty thinking tags should be removed
  it('Empty thinking tags should be removed', () => {
    // Create message with empty thinking tag
    const message = `Hello world!
<think>
</think>

More content...`;

    // Create processor with streaming=true
    const processor = new ThinkTagProcessor(message, true);
    
    // Process the message
    const result = processor.processThinkingTags();

    // The empty thinking tag should be removed
    expect(result).not.toContain('think-container');
  });
  
  // Test case 6: Multiple thinking tags with different states
  it('Multiple thinking tags with different states - only unclosed should glow during streaming', () => {
    const message = `Hello world!
<think>
This is a closed thought
</think>

More content...

<think>
This is an unclosed thought
`;

    const processor = new ThinkTagProcessor(message, true);
    const result = processor.processThinkingTags();
    
    // Should find both containers but only one should have the thinking class
    const matches = result.match(/think-container/g);
    expect(matches?.length).toBe(2);
    
    const thinkingMatches = result.match(/think-container thinking/g);
    expect(thinkingMatches?.length).toBe(1);
  });
  
  // Test case 7: Nested think tags should be processed correctly
  it('Nested think tags should be processed correctly', () => {
    const message = `Hello world!
<think>
Outer thinking
<think>
Inner thinking
</think>
More outer thinking
</think>`;

    const processor = new ThinkTagProcessor(message, false);
    const result = processor.processThinkingTags();
    
    // Should properly contain both inner and outer content
    expect(result).toContain('Outer thinking');
    expect(result).toContain('Inner thinking');
    expect(result).toContain('More outer thinking');
    
    // NOTE: Our simple implementation doesn't handle nested tags separately
    // It processes them as a single think tag with the content of both
    // This is consistent with the actual implementation in MessageProcessor
    const matches = result.match(/think-container/g);
    expect(matches?.length).toBe(1);
  });
}); 