# Requirements: Zed Token Limit Error Investigation

## Background

User is running a custom Zed fork and experiencing "prompt is too long: 205507 tokens > 200000 maximum" errors from Anthropic's API. This is a known issue in Zed related to inaccurate client-side token counting.

## User Stories

### US-1: Cherry-pick Token Counting Fix
As a user running a custom Zed fork, I want to cherry-pick the improved token counting changes from upstream so that I can avoid exceeding Anthropic's token limits without doing a full upstream merge.

## Acceptance Criteria

### AC-1: Identify Required Commits
- [ ] Document the specific commit hash for PR #44943 (`d16619a654`)
- [ ] List all files changed in that commit
- [ ] Identify any dependent commits that may be required

### AC-2: Document Cherry-pick Process
- [ ] Provide step-by-step instructions to cherry-pick the fix
- [ ] Note any potential merge conflicts to expect
- [ ] List any Cargo.toml dependency changes needed

### AC-3: Verify Fix Works
- [ ] Token counts should now use Anthropic's `/v1/messages/count_tokens` API
- [ ] Token display should reflect actual usage from LLM responses
- [ ] Requests should no longer be sent when estimated tokens exceed model limits

## Relevant GitHub Issues

| Issue | Description |
|-------|-------------|
| #38533 | Token count accuracy (closed by PR #44943) |
| #31854 | Context limit display mismatch (200K vs 120K) |
| #34486 | Commit message generation exceeding token limit |
| #32150 | Thread reached token limit discussion |

## Key Commit

**PR #44943**: `d16619a654` - "Improve token count accuracy using Anthropic's API"
- Merged: December 16, 2025
- Author: Richard Feldman