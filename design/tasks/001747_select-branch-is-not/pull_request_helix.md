# Sort branch dropdowns alphabetically on NewSpecTaskForm

## Summary
The "Base branch" and "Select existing branch" dropdowns on the new spec task form displayed branches in unsorted order, making it hard to find branches in repos with many branches. Added case-insensitive alphabetical sorting to both dropdowns.

## Changes
- Added `.slice().sort()` with `localeCompare` to the "Base branch" dropdown
- Added `.sort()` with `localeCompare` to the "Select existing branch" dropdown (after the existing `.filter()`)
