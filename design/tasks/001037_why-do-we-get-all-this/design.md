# Design: Markdown Placeholder Leakage Fix

## Architecture

The fix targets `frontend/src/components/session/Markdown.tsx`, specifically the `MessageProcessor` class and the `InteractionMarkdown` component.

## Problem Analysis

There are **two distinct bugs** causing placeholders to leak into rendered output:

### Bug 1: CITATION_DATA extraction failure

**Location**: `processContent()` in `InteractionMarkdown` component (lines 784-795)

```typescript
const citationPattern = /__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/;
const citationDataMatch = content.match(citationPattern);
if (citationDataMatch) {
  try {
    const citationDataJson = citationDataMatch[1];
    const data = JSON.parse(citationDataJson);
    setCitationData(data);
    content = content.replace(/__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/, '');
  } catch (error) {
    console.error('Error parsing citation data:', error);
    setCitationData(null);
    // BUG: markers NOT removed from content here!
  }
}
```

**Issue**: When JSON parsing fails, the catch block does NOT remove the markers from content. The markers then pass through to react-markdown, which interprets `__text__` as bold syntax and consumes the underscores.

### Bug 2: INLINE_CODE restore failure

**Location**: `sanitizeHtml()` in `MessageProcessor` class (lines 500-502)

```typescript
inlineCode.forEach((code, index) => {
  processedMessage = processedMessage.replace(`__INLINE_CODE_${index}__`, code);
});
```

**Issue**: The restore loop uses `.replace()` which only replaces the first occurrence. The exact failure mode needs investigation - possible causes:
- DOMPurify modifying placeholder text in some edge cases
- Mismatch between array indices and placeholder numbers
- Race condition during streaming updates

## Solution: Two-Pronged Approach

### Fix 1: Always remove markers (even on error)

Move marker removal outside the try/catch:

```typescript
const citationPattern = /__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/;
const citationDataMatch = content.match(citationPattern);
if (citationDataMatch) {
  // Always remove markers first
  content = content.replace(citationPattern, '');
  try {
    const citationDataJson = citationDataMatch[1];
    const data = JSON.parse(citationDataJson);
    setCitationData(data);
  } catch (error) {
    console.error('Error parsing citation data:', error);
    setCitationData(null);
  }
}
```

### Fix 2: Use non-markdown placeholder syntax

Replace `__PLACEHOLDER__` with `\x00PLACEHOLDER\x00` (null byte delimiters):

```typescript
// Before (collides with markdown bold syntax)
`__INLINE_CODE_${index}__`
`__CITATION_DATA__${json}__CITATION_DATA__`

// After (cannot appear in valid text, no markdown interpretation)
`\x00INLINE_CODE_${index}\x00`
`\x00CITATION_DATA\x00${json}\x00/CITATION_DATA\x00`
```

**Why null bytes?**
- Cannot appear in valid UTF-8 text content
- No markdown interpretation possible
- If they somehow leak through, they render as invisible/empty rather than confusing text
- Easy regex: `/\x00INLINE_CODE_(\d+)\x00/g`

**Alternative considered**: `<<<PLACEHOLDER>>>` - rejected because `<` and `>` could interact with HTML parsing in DOMPurify.

## Implementation Plan

### Phase 1: Fix the error handling bug (immediate)

This is the most likely cause of the CITATION_DATA leak:

```typescript
// In processContent(), move removal before try/catch
if (citationDataMatch) {
  content = content.replace(citationPattern, '');  // Remove first!
  try {
    // ... parse and use
  } catch {
    // ... handle error, markers already removed
  }
}
```

### Phase 2: Change placeholder syntax (defensive)

Update all placeholder patterns to use null bytes:

| Location | Old Pattern | New Pattern |
|----------|-------------|-------------|
| `sanitizeHtml()` | `__CODE_BLOCK_${i}__` | `\x00CB${i}\x00` |
| `sanitizeHtml()` | `__INLINE_CODE_${i}__` | `\x00IC${i}\x00` |
| `addCitationData()` | `__CITATION_DATA__` | `\x00CD\x00` |
| `processThinkingTags()` | `__THINKING_WIDGET__` | `\x00TW\x00` |
| `processContent()` | All extraction patterns | Match new patterns |

## Files to Change

- `frontend/src/components/session/Markdown.tsx`:
  - `sanitizeHtml()`: Update placeholder patterns and restore logic
  - `addCitationData()`: Update marker format
  - `processThinkingTags()`: Update marker format  
  - `processContent()`: Fix error handling, update extraction patterns

- `frontend/src/components/session/MessageProcessor.test.tsx`:
  - Update pattern assertions to match new syntax
  - Add test for error handling (JSON parse failure should not leak markers)

## Risks

- **Low**: Null bytes are safe - they can't appear in valid content and won't render if leaked
- **Testing**: Run full test suite and manual streaming test after changes