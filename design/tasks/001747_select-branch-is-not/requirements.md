# Requirements: Sort Branch Dropdowns on New SpecTask Form

## User Story

As a user creating a new spec task, I want the branch selection dropdowns to be sorted alphabetically so I can quickly find the branch I need.

## Problem

The "Base branch" and "Select branch" dropdowns on the New SpecTask form display branches in an unsorted order (whatever order git returns them). This makes it hard to find branches, especially in repos with many branches.

## Acceptance Criteria

- [ ] Branches in the "Base branch" dropdown are sorted alphabetically (case-insensitive)
- [ ] Branches in the "Select existing branch" dropdown are sorted alphabetically (case-insensitive)
- [ ] The default branch keeps its `(default)` chip label and remains visually identifiable regardless of sort position
- [ ] Sorting is consistent — same order every time the dropdown loads
