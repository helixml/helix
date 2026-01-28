# Implementation Tasks

## Investigation

- [ ] Verify the user's current fork commit hash to assess how far behind upstream it is
- [ ] Check if commit `d16619a654` applies cleanly to the user's fork

## Cherry-pick the Fix

- [ ] Fetch upstream Zed repository: `git fetch upstream`
- [ ] Cherry-pick the token counting fix: `git cherry-pick d16619a654`
- [ ] Resolve any merge conflicts in affected files:
  - `crates/anthropic/src/anthropic.rs`
  - `crates/language_models/src/provider/anthropic.rs`
  - `crates/agent/src/thread.rs`
  - `crates/language_models/src/provider/cloud.rs`

## Build and Test

- [ ] Run `cargo check` to verify compilation
- [ ] Run `cargo test -p anthropic` to verify Anthropic crate tests pass
- [ ] Run `cargo test -p language_models` to verify language model tests pass
- [ ] Run `cargo test -p agent` to verify agent tests pass

## Verification

- [ ] Build Zed: `cargo build --release`
- [ ] Test with a long context conversation that previously failed
- [ ] Verify token count display in UI reflects actual usage from API responses
- [ ] Confirm no more "prompt is too long" errors when staying within displayed limits

## Fallback (if cherry-pick fails)

- [ ] Manually apply the `CountTokensRequest` and `count_tokens()` additions to `crates/anthropic/src/anthropic.rs`
- [ ] Update the `count_tokens` trait implementation in `crates/language_models/src/provider/anthropic.rs`
- [ ] Or: reduce displayed context limit to 180K as a safety buffer workaround