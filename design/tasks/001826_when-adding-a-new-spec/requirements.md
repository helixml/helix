# Requirements: Duplicate Task Detection

## User Story

As a user creating a new spec task, I want to be warned if a similar task already exists so I can avoid duplicates and instead add context to the existing task.

## Acceptance Criteria

1. **Duplicate search on create**: When the user clicks "Create task", the system searches existing tasks in the same project for similar prompts using Postgres full-text search (BM25-style ranking via `ts_rank`).

2. **Match display**: If matches are found (score above threshold), a dialog shows before creating:
   - "Is this a duplicate of...?" with a list of matching tasks (name, status, score)
   - Each match is clickable to view the full task

3. **Comment on duplicate**: The user can choose to add their prompt as a comment on an existing task instead of creating a new one. This requires a new general-purpose task comments system (the existing `SpecTaskDesignReviewComment` is scoped to design reviews only).

4. **Create anyway**: The user can dismiss the duplicates dialog and proceed with task creation as normal.

5. **Scope**: Duplicate search is scoped to the current project. Archived tasks should be excluded.

## Out of Scope

- Cross-project duplicate detection
- Semantic/embedding-based search (BM25 via Postgres `tsvector` is sufficient for v1)
- Merging duplicate tasks
- Blocking duplicate creation (advisory only)
