# Design: Fix Blinker HTML Tag Visibility During Streaming

## Overview

The blinker span (`<span class="blinker-class">┃</span>`) renders as literal text in monospace font during streaming, indicating it's being incorrectly placed inside `<pre><code>` context when adjacent to code blocks.

## Architecture

The current rendering pipeline:
1. `MessageProcessor.process()` transforms raw markdown content
2. `sanitizeHtml()` uses DOMPurify to clean content
3. `addBlinker()` appends blinker span to raw markdown (if streaming)
4. `react-markdown` with `rehype-raw` converts markdown + raw HTML to rendered output

The problem: The blinker span is appended to raw markdown, then `react-markdown` + `rehype-raw` processes it. When a code block precedes the blinker, the `rehype-raw` plugin may incorrectly nest the span inside the generated `<pre><code>` tags.

## Solution Options

### Option A: Inline Blinker (Keep Current UX, Fix Bug)

Keep the inline cursor behavior but ensure it renders correctly by adding a newline or delimiter before the blinker span to prevent `rehype-raw` from nesting it in code blocks:

```typescript
private addBlinker(message: string): string {
  const openCodeBlockCount = (message.match(/```/g) || []).length;
  if (openCodeBlockCount % 2 !== 0) {
    return message;
  }

  // Add newline after code blocks to separate blinker from pre/code context
  if (message.trimEnd().endsWith('```')) {
    return message + '\n\n<span class="blinker-class">┃</span>';
  }

  return message + '<span class="blinker-class">┃</span>';
}
```

**Pros**: Keeps inline cursor after regular text
**Cons**: May still have edge cases; blinker on new line after code blocks

### Option B: Replace with Streaming Spinner (Better UX)

Replace the inline blinker with a spinner/loading indicator on its own line. Render as a sibling React element after the Markdown component:

```tsx
return (
  <>
    <Markdown
      children={processedContent}
      remarkPlugins={remarkPluginsArray}
      rehypePlugins={rehypePluginsArray}
      className="interactionMessage"
      components={markdownComponents}
    />
    {showBlinker && isStreaming && (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1 }}>
        <CircularProgress size={16} />
      </Box>
    )}
  </>
);
```

**Pros**: 
- Eliminates the bug entirely (blinker never in markdown)
- Modern UX (similar to ChatGPT's pulsing dots)
- Cleaner separation of concerns

**Cons**: 
- Visual change from inline cursor to below-content spinner
- Spinner always on its own line, not inline after text

### Recommended: Option B (Spinner)

The spinner approach is cleaner and completely eliminates the parsing issue. The current inline blinker is a nice touch but causes rendering bugs. A spinner below content is a well-established pattern for streaming AI responses.

## Changes Required

1. **Remove** `addBlinker()` call from `MessageProcessor.process()`
2. **Remove** the `addBlinker()` method 
3. **Add** spinner/loading indicator as sibling element after `<Markdown>` in `MemoizedMarkdownRenderer`
4. **Pass** `showBlinker` and `isStreaming` props to the renderer component
5. **Choose spinner style**: MUI `CircularProgress`, pulsing dots, or similar

## Spinner Style Options

| Style | Description |
|-------|-------------|
| `CircularProgress` (small) | Simple spinning circle |
| Pulsing dots (●●●) | Three dots that pulse in sequence (ChatGPT style) |
| Typing indicator | "AI is typing..." text with animation |

## Testing Strategy

- Unit test: Spinner appears during streaming, disappears when done
- Unit test: No blinker HTML in rendered markdown output
- Manual test: Spinner animation works correctly
- Manual test: Spinner positioning looks good after various content types (text, code, lists)