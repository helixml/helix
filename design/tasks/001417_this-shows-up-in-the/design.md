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

## Solution: Add Blinker After Markdown Rendering

Instead of appending the blinker to raw markdown (before parsing), inject it after the markdown-to-HTML conversion is complete.

### Option A: Move Blinker to React Component Layer

The cleanest fix is to render the blinker as a separate React element, not as raw HTML injected into markdown.

In `InteractionMarkdown` or `MemoizedMarkdownRenderer`:

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
      <span className="blinker-class">┃</span>
    )}
  </>
);
```

This ensures the blinker is always a sibling element after the rendered markdown, never inside it.

### Option B: Use CSS ::after Pseudo-element

Add the blinker via CSS on a wrapper element when streaming:

```tsx
<Box className={isStreaming ? 'streaming-content' : ''}>
  <Markdown ... />
</Box>
```

```css
.streaming-content::after {
  content: '┃';
  animation: blink 1.2s step-end infinite;
}
```

### Recommended: Option A

Option A is simpler and maintains the existing blinker styling. It requires minimal changes to the component structure.

## Changes Required

1. **Remove** `addBlinker()` call from `MessageProcessor.process()`
2. **Remove** the `addBlinker()` method (or keep for reference)
3. **Add** blinker rendering in `MemoizedMarkdownRenderer` as a sibling element
4. **Pass** `showBlinker` and `isStreaming` props to the renderer component

## Alternatives Considered

| Alternative | Why Rejected |
|-------------|--------------|
| Close unclosed tags before adding blinker | Doesn't fix the core issue - `rehype-raw` interaction with code blocks |
| Escape blinker HTML in code blocks | Complex regex, fragile |
| Use different blinker character | Doesn't solve the rendering context issue |

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Render blinker in React, not raw HTML | Avoids markdown parser interference entirely |
| Place blinker as sibling after Markdown | Guarantees it's never inside code/pre blocks |
| Keep existing blinker CSS | No visual changes to users |

## Testing Strategy

- Unit test: Blinker appears after code block (not inside)
- Unit test: Blinker appears during streaming, disappears when done
- Unit test: Multiple code blocks don't affect blinker position
- Manual test: Verify blinker animation works correctly
- Manual test: Verify blinker positioning at end of various content types