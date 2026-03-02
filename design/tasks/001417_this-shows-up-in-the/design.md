# Design: Fix Blinker HTML Tag Visibility During Streaming

## Overview

The blinker span (`<span class="blinker-class">┃</span>`) occasionally renders as literal text during streaming when content contains unclosed HTML tags. We need to ensure the blinker always renders properly regardless of the content state.

## Architecture

The current rendering pipeline:
1. `MessageProcessor.process()` transforms raw markdown/HTML content
2. `sanitizeHtml()` uses DOMPurify to clean content
3. `addBlinker()` appends blinker span (if streaming)
4. `react-markdown` with `rehype-raw` renders final HTML

The problem: `addBlinker()` runs **after** sanitization, so unclosed tags in the output can "swallow" the blinker span.

## Solution: Close Unclosed HTML Tags Before Adding Blinker

Add a tag-closing step before appending the blinker. This ensures the blinker is always appended to valid HTML.

### Implementation

In `Markdown.tsx`, modify `addBlinker()`:

```typescript
private addBlinker(message: string): string {
  // Existing code block check
  const openCodeBlockCount = (message.match(/```/g) || []).length;
  if (openCodeBlockCount % 2 !== 0) {
    return message;
  }

  // NEW: Close any unclosed HTML tags before adding blinker
  message = this.closeUnclosedTags(message);

  return message + '<span class="blinker-class">┃</span>';
}

private closeUnclosedTags(html: string): string {
  // Track opened tags (only self-closing-optional tags that matter for streaming)
  const tagStack: string[] = [];
  const tagRegex = /<\/?([a-zA-Z][a-zA-Z0-9]*)[^>]*>/g;
  const selfClosingTags = new Set(['br', 'hr', 'img', 'input', 'meta', 'link']);
  
  let match;
  while ((match = tagRegex.exec(html)) !== null) {
    const [fullMatch, tagName] = match;
    const tag = tagName.toLowerCase();
    
    if (selfClosingTags.has(tag)) continue;
    
    if (fullMatch.startsWith('</')) {
      // Closing tag - pop from stack if matching
      const lastIndex = tagStack.lastIndexOf(tag);
      if (lastIndex !== -1) {
        tagStack.splice(lastIndex, 1);
      }
    } else if (!fullMatch.endsWith('/>')) {
      // Opening tag
      tagStack.push(tag);
    }
  }
  
  // Check for partial/unclosed tag at end (e.g., "<div" or "<span class=")
  const partialTagMatch = html.match(/<([a-zA-Z][a-zA-Z0-9]*)[^>]*$/);
  if (partialTagMatch) {
    // Remove the partial tag - it will be completed in next streaming chunk
    html = html.replace(/<([a-zA-Z][a-zA-Z0-9]*)[^>]*$/, '');
  }
  
  // Close remaining open tags in reverse order
  for (let i = tagStack.length - 1; i >= 0; i--) {
    html += `</${tagStack[i]}>`;
  }
  
  return html;
}
```

## Alternatives Considered

1. **Move blinker before sanitization**: Rejected - DOMPurify might strip or modify the blinker
2. **Use CSS ::after pseudo-element**: Rejected - requires DOM structure changes, harder to position
3. **Add blinker via React state**: Rejected - would require significant refactor of MessageProcessor

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Close tags in `addBlinker()` | Minimal change, fixes problem at source |
| Remove partial tags at end | Prevents malformed HTML, chunk will complete it |
| Only track non-self-closing tags | Self-closing tags don't affect blinker rendering |

## Testing Strategy

- Unit test: Streaming with unclosed `<div>` tag
- Unit test: Streaming with partial tag `<span class="`
- Unit test: Verify existing code block handling unchanged
- Unit test: Multiple unclosed tags at different nesting levels