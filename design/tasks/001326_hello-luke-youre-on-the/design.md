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

2. **Add Discord invitation:** Add a Discord call-to-action with a primary button that links to the Discord server.

## Key Decisions

**GitHub text change:** 
- Current: "Alternatively, you can deploy Helix yourself, source code is available in our GitHub repo"
- New: "Alternatively, you can download and deploy Helix yourself from our GitHub repo"

This removes the "source code" phrasing which implies compiling from source, making it sound more accessible.

**Discord styling:** Use a primary MUI `Button` component with `variant="contained"` to make it a strong call-to-action. Style it with the accent color (`#00e891`) to match the page's visual theme. The button should link to Discord and open in a new tab.

**Discord placement:** Add after the "We're gradually rolling out access..." text and before the email display. The button gives the Discord CTA visual prominence.

**Discord text:** Button text: "Join our Discord" with supporting text above it: "Introduce yourself and get faster access"

## Links

- Discord: https://discord.gg/VJftd844GE
- GitHub: https://github.com/helixml/helix (existing)