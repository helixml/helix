# Implementation Tasks: Find AI Rosie and Jim Public Matching Agents First Pass

## Phase 1 — Foundations (internal tier only, nothing public)

- [ ] Create the `match_index` schema: `candidate_profile`, `job_spec`, `interest_record`
- [ ] Create the two DB roles: `findos_sync` (read/write, holds Bullhorn creds) and `public_match_ro` (SELECT on published-only views, INSERT on `interest_record`)
- [ ] Create the public-facing views that omit `bullhorn_id`, `consent_*` and hard-filter `published = true`
- [ ] Verify `public_match_ro` cannot reach any identifying column — write a test that asserts this
- [ ] Build the Bullhorn → `job_spec` sync job (read-only, scheduled, AI/ML specialism tag filter)
- [ ] Add the Interest queue section to Find OS (list, statuses: new / contacted / introduced / closed)
- [ ] Wire Interest creation to a Slack alert in the existing Find AI channel, with a deep link
- [ ] Extend Find OS SSO to allow `@we-find.ai` addresses alongside `@linuxrecruit.co.uk`
- [ ] Confirm Bullhorn API read access is live and rate limits are understood

## Phase 2 — Rosie (candidate side)

- [ ] CV upload endpoint accepting PDF and DOCX, with a paste-text fallback
- [ ] `parse_document` tool (the only tool the ingest agents get)
- [ ] Profile Builder agent: CV → structured profile, with the anonymisation rules in the system prompt
- [ ] Anonymisation post-check: assert no employer names, no contact details, no personal URLs before write
- [ ] Candidate draft-profile review UI with per-field editing
- [ ] Consent capture on approve: timestamp, IP, hash of exact approved text
- [ ] Magic-link (passwordless) return flow for profile management
- [ ] Unpublish and delete, both taking effect on the next search with no cache lag
- [ ] Generate and store embeddings on profile publish

## Phase 3 — Jim (client side)

- [ ] Client account signup + consultant approval flow in Find OS (no self-serve access to candidates)
- [ ] Job Intake agent: JD upload / pasted advert / short form → structured spec
- [ ] Client review-and-correct UI for the parsed spec
- [ ] `search_candidates` tool: hybrid retrieval (vector recall ~100, then LLM scoring of top N)
- [ ] Matcher agent producing ranked cards with separate skill-overlap and LLM-fit components plus a plain-English reason
- [ ] Results UI rendering `Candidate #N` cards only
- [ ] "Register interest" action writing an `interest_record` (the public tier's only write)
- [ ] Candidate notification on interest, sent by Find OS, not naming the client
- [ ] Consultant view: open an Interest, see the de-anonymised candidate and client
- [ ] Human-gated programmatic promotion of a candidate into Bullhorn (no agent writes)

## Phase 4 — Hardening and seeding

- [ ] Wrap all user-supplied text as delimited untrusted content in every agent prompt
- [ ] Prompt-injection test suite: malicious CV and malicious JD must fail closed, never disclose
- [ ] Per-account and per-IP rate limits on requests
- [ ] Per-account LLM token/spend caps, failing cleanly with a message rather than burning credit
- [ ] Route ingest agents to a cheap model; reserve the expensive model for Matcher
- [ ] Audit log: every match run, every card rendered, every Interest raised — who, when, inputs, outputs, cost
- [ ] Confirm the public tier process has no Bullhorn credentials in its environment
- [ ] Security review pass over the public tier before launch
- [ ] Build the consent-campaign export from Bullhorn (AI/ML subset) with unique signup links
- [ ] Run the seeding campaign and onboard friendly beta candidates and clients

## Phase 3 cut line

- [ ] R3 (candidate-side job matching via Reverse Matcher) — build if week 3 holds, cut first if it slips

## Follow-up outside this spec

- [ ] Fix the spec-task attachment upload path so same-named files de-duplicate instead of overwriting (six of seven Find AI transcripts were silently clobbered; recovered from git history into `attachments/`)
