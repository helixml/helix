# Design: Zed Token Counting Fix

## Problem Summary

Zed's client-side token estimation uses GPT-4's tiktoken tokenizer as a fallback for Anthropic models, which can be 15-30% inaccurate. This causes prompts to be sent that exceed Anthropic's 200K token limit.

## Architecture

### Current Token Flow (Pre-fix)

```
User Input → tiktoken (GPT-4 tokenizer) → Estimate → Send to Anthropic → REJECT (too long)
```

### Fixed Token Flow (PR #44943)

```
User Input → Anthropic /v1/messages/count_tokens API → Accurate Count → UI Display
           → LLM Response includes actual usage → Update Thread Token Display
```

## Key Components Changed

### 1. `crates/anthropic/src/anthropic.rs`
- Added `CountTokensRequest` and `CountTokensResponse` structs
- Added `count_tokens()` async function to call Anthropic's token counting API
- Endpoint: `POST {api_url}/v1/messages/count_tokens`

### 2. `crates/language_models/src/provider/anthropic.rs`
- Added `into_anthropic_count_tokens_request()` to convert Zed's request format
- Updated `count_tokens` implementation to use Anthropic's API instead of tiktoken
- Properly converts all message types (text, images, tool use, tool results)

### 3. `crates/agent/src/thread.rs`
- Token usage now tracked from actual LLM response (`UsageUpdate` events)
- `request_token_usage` HashMap stores per-message token usage
- `latest_token_usage()` returns real usage from most recent request

## Why Tiktoken Was Inaccurate

1. **Different tokenizer**: Claude uses its own tokenizer, not OpenAI's
2. **Missing content types**: Tool uses weren't counted at all (TODO comment in old code)
3. **Image estimation**: Only rough estimates for image tokens
4. **System prompt handling**: Different token overhead between models

## Cherry-pick Strategy

The fix is self-contained in commit `d16619a654`. Files affected:

| File | Changes |
|------|---------|
| `crates/anthropic/src/anthropic.rs` | +65 lines (new API types and function) |
| `crates/language_models/src/provider/anthropic.rs` | +240/-60 lines (major refactor) |
| `crates/agent/src/thread.rs` | +22 lines (track actual usage) |
| `crates/agent/src/tests/mod.rs` | +178 lines (tests) |
| `crates/language_models/src/provider/cloud.rs` | +10/-1 lines |

## Potential Conflicts

- If your fork has modified any of these files, expect conflicts
- The `anthropic.rs` changes are additive and should apply cleanly
- The `provider/anthropic.rs` refactor is substantial and may conflict

## Alternative Workaround

If cherry-picking is too complex, a simpler workaround is to reduce the displayed context limit in your fork's UI to provide a safety buffer (e.g., show 180K instead of 200K).