# Diff viewer comments

## Symptom

The diff viewer showed the comment affordance, but completing the gutter gesture did not open a comment draft on Meta and Prime.

## Root causes

- `onSelectedLinesChange` updated React state during Pierre's pointer gesture and interrupted its document-level pointer-up handling.
- Pierre mutates `fileDiff.cacheKey` after parsing, so using it as the item ID detached drafts from their diff item.
- Annotation changes reused the same item without a new `version`, so Pierre did not render the draft annotation.

## Fix

Use the stable `${path}:${index}` item ID, stop passing `onSelectedLinesChange`, and increment the item version when annotations change. The existing shared comment callback then handles the saved comment.

## Prime verification

On Prime, after rebuilding the frontend and API, adding, saving, deleting, and immediately reopening a diff comment all worked. The focused test passed with 10 tests, and the production frontend build transformed 22,323 modules successfully.
