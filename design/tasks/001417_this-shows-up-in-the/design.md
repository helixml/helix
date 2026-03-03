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

## Streaming Indicator Style Options

### 1. Pulsing Dots (ChatGPT style) ⭐ User's Preference
Three dots that bounce/pulse in sequence. Already exists in codebase as `ThinkingBox` in `InteractionLiveStream.tsx`.

```tsx
// Bouncing dots with staggered delays
<Box sx={{ display: 'flex', gap: '4px' }}>
  {[0, 0.2, 0.4].map((delay, i) => (
    <Box key={i} sx={{
      width: 8, height: 8, borderRadius: '50%',
      backgroundColor: '#999',
      animation: 'bounce 1.4s ease-in-out infinite',
      animationDelay: `${delay}s`,
    }} />
  ))}
</Box>
```

### 2. Gradient Wave Bar ✨ Sexy Option
An animated gradient bar that shimmers like a progress wave. Similar to the streaming citation indicator already in `Citation.tsx`.

```tsx
const shimmer = keyframes`
  0% { background-position: 200% center; }
  100% { background-position: -200% center; }
`;

<Box sx={{
  height: 3,
  width: 60,
  borderRadius: 2,
  background: 'linear-gradient(90deg, transparent, #00D4FF, #7C3AED, #00D4FF, transparent)',
  backgroundSize: '200% 100%',
  animation: `${shimmer} 1.5s ease-in-out infinite`,
}} />
```

### 3. Helix DNA Spinner 🧬 On-Brand Option
Two interweaving dots that rotate in a helix pattern - ties into the Helix brand.

```tsx
const helixSpin = keyframes`
  0% { transform: rotateY(0deg); }
  100% { transform: rotateY(360deg); }
`;

<Box sx={{ 
  perspective: '100px',
  animation: `${helixSpin} 1.2s linear infinite`,
}}>
  <Box sx={{ display: 'flex', gap: 1 }}>
    <Box sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: '#00D4FF' }} />
    <Box sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: '#7C3AED' }} />
  </Box>
</Box>
```

### 4. Typing Cursor with Gradient Glow 💫 Modern Option
A vertical cursor bar with an animated gradient glow - more modern take on the original blinker.

```tsx
const glowPulse = keyframes`
  0%, 100% { box-shadow: 0 0 8px 2px rgba(0, 212, 255, 0.6); }
  50% { box-shadow: 0 0 16px 4px rgba(124, 58, 237, 0.8); }
`;

<Box sx={{
  width: 2,
  height: 20,
  borderRadius: 1,
  background: 'linear-gradient(180deg, #00D4FF, #7C3AED)',
  animation: `${glowPulse} 1s ease-in-out infinite, blink 1s step-end infinite`,
}} />
```

### 5. Audio Waveform Bars 🎵 Playful Option
Three bars that animate like an audio equalizer - suggests active processing.

```tsx
<Box sx={{ display: 'flex', alignItems: 'flex-end', gap: '2px', height: 16 }}>
  {[0, 0.2, 0.4].map((delay, i) => (
    <Box key={i} sx={{
      width: 3,
      backgroundColor: '#00D4FF',
      borderRadius: 1,
      animation: `wave 0.8s ease-in-out infinite`,
      animationDelay: `${delay}s`,
      '@keyframes wave': {
        '0%, 100%': { height: 4 },
        '50%': { height: 16 },
      },
    }} />
  ))}
</Box>
```

### Recommendation

**Primary choice**: Option 2 (Gradient Wave Bar) or Option 4 (Typing Cursor with Glow) - both are sleek and modern.

**Fallback**: Option 1 (Pulsing Dots) - proven pattern, already in codebase.

The gradient wave bar is particularly sexy because it:
- Uses Helix brand colors (#00D4FF cyan, #7C3AED purple)
- Has smooth, hypnotic motion
- Is subtle but clearly indicates activity
- Matches the existing shimmer effect used in citations

## Testing Strategy

- Unit test: Spinner appears during streaming, disappears when done
- Unit test: No blinker HTML in rendered markdown output
- Manual test: Spinner animation works correctly
- Manual test: Spinner positioning looks good after various content types (text, code, lists)