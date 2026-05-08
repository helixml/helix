# Fix unreadable link colors in spec review and design doc pages

## Summary
Links in the spec review (`DesignReviewContent.tsx`) and public design doc page (`DesignDocPage.tsx`) were rendering in browser-default dark blue (`#0000EE`) on a dark `background.paper` background, making them nearly unreadable (~1.4:1 contrast ratio). Added explicit link styling using Helix teal (`#00d5ff`), matching the existing pattern in `CodeIntelligenceTab.tsx`.

## Changes
- Added `& a` CSS rule to `.markdown-body` in `DesignReviewContent.tsx` with color `#00d5ff`, no text-decoration, underline on hover, and pinned visited color
- Added matching `& a` CSS rule to the markdown container in `DesignDocPage.tsx`
