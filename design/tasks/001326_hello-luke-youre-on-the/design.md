# Design: Add Discord Invitation to Waitlist Page

## Overview

Add a Discord invitation message to the waitlist page to encourage users to join the community for faster access.

## Location

**File:** `helix/frontend/src/pages/Waitlist.tsx`

The waitlist page is a simple React component using MUI (Material-UI) that displays a message to users who are on the waitlist.

## Architecture

No architectural changes needed. This is a text/UI addition to an existing component.

## Implementation Approach

Add a new `Typography` block after the existing "We're gradually rolling out access..." paragraph. The new block will contain:
1. The Discord invitation message
2. A clickable link to the Discord server

## Key Decisions

**Styling:** Match the existing link style used for the GitHub repo link (inline `<a>` tag with `target="_blank"` and `rel="noopener noreferrer"`).

**Placement:** Add as a separate paragraph between the "We're gradually rolling out access..." text and the email display. This gives the Discord CTA visual prominence while keeping the page flow logical.

**Text:** "Join our Discord, introduce yourself, and get faster access." - Direct and actionable.

## Discord Link

```
https://discord.gg/VJftd844GE
```
