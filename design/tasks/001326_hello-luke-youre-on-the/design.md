# Design: Add Discord Invitation to Waitlist Page

## Overview

Add a Discord invitation message to the waitlist page to encourage users to join the community for faster access. Also update the GitHub text to be more welcoming.

## Location

**File:** `helix/frontend/src/pages/Waitlist.tsx`

The waitlist page is a simple React component using MUI (Material-UI) that displays a message to users who are on the waitlist.

## Architecture

No architectural changes needed. This is a text/UI addition to an existing component.

## Implementation Approach

1. **Update GitHub text:** Change "source code is available in our GitHub repo" to friendlier wording like "you can download it from our GitHub repo" - this sounds less intimidating than suggesting users need to compile from source.

2. **Add Discord invitation:** Add a new `Typography` block after the existing "We're gradually rolling out access..." paragraph containing the Discord invitation message with a clickable link.

## Key Decisions

**GitHub text change:** 
- Current: "Alternatively, you can deploy Helix yourself, source code is available in our GitHub repo"
- New: "Alternatively, you can download and deploy Helix yourself from our GitHub repo"

This removes the "source code" phrasing which implies compiling from source, making it sound more accessible.

**Discord styling:** Match the existing link style used for the GitHub repo link (inline `<a>` tag with `target="_blank"` and `rel="noopener noreferrer"`).

**Discord placement:** Add as a separate paragraph between the "We're gradually rolling out access..." text and the email display. This gives the Discord CTA visual prominence while keeping the page flow logical.

**Discord text:** "Join our Discord, introduce yourself, and get faster access." - Direct and actionable.

## Links

- Discord: https://discord.gg/VJftd844GE
- GitHub: https://github.com/helixml/helix (existing)