# Requirements: Shareable Design Doc Page Not Scrollable

## Problem Statement

When users click on a shareable design doc link (e.g., `/design-doc/:specTaskId/:reviewId`), the page content is not scrollable. This makes it impossible to read design documents that extend beyond the viewport height.

## User Stories

1. **As a user viewing a shared design doc link**, I want to scroll through the entire document so I can read all the content.

2. **As a reviewer**, I want to view shared design specs on any device and be able to scroll through long requirements, technical designs, or implementation plans.

## Acceptance Criteria

- [ ] The `/design-doc/:specTaskId/:reviewId` page allows vertical scrolling when content exceeds viewport height
- [ ] All three document tabs (Requirements, Technical Design, Implementation Plan) are scrollable
- [ ] The header with task name and navigation remains visible while scrolling content
- [ ] Scrolling works correctly on both desktop and mobile browsers
- [ ] No horizontal scrollbar appears unless content requires it (e.g., wide code blocks)

## Root Cause Analysis

The `DesignDocPage.tsx` component renders a `Container` directly without wrapping it in the `Page` component. The app's `Layout.tsx` sets `overflow: 'hidden'` on the main content area, expecting child pages to manage their own scroll behavior. The `Page` component provides `overflowY: 'auto'` on its content area, but `DesignDocPage` bypasses this.