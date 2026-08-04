# Design: Find AI Rosie and Jim Public Matching Agents First Pass

## 1. The central architectural decision

Helix's agent pattern — proven daily in Find OS and in `chris-outreach` — is: **give the agent its
own computer**. A sandbox with a Linux desktop, a real Chrome, a shell, a git repo of markdown
memory, and a human who logs into things for it. It is extremely capable precisely because it is
unconstrained.

That pattern must **not** be extended to public traffic. The first pass therefore has two tiers with
a hard boundary between them:

```
┌─────────────────────────────────────────────────────────────────────┐
│ PUBLIC TIER — anonymous + verified-client traffic                   │
│ we-find.ai  (Helix web service, prod SaaS, prj_01kvz0e7b401545…)    │
│                                                                     │
│  Rosie (candidate)          Jim (client)                            │
│  ├ Profile Builder agent    ├ Job Intake agent                      │
│  └ Reverse Matcher agent    └ Matcher agent                         │
│                                                                     │
│  • stateless LLM calls, fixed tool allowlist                        │
│  • NO sandbox / desktop / browser / shell / LinkedIn                │
│  • reads ONLY the Match Index, via a SELECT-only DB role            │
│  • holds NO Bullhorn credentials                                    │
└───────────────────────────┬─────────────────────────────────────────┘
                            │ Match Index (read-only view)
                            │ Interest records (append-only write)
┌───────────────────────────┴─────────────────────────────────────────┐
│ INTERNAL TIER — authenticated Find AI / Linuxrecruit staff only     │
│ os.we-find.ai  (Find OS, Google SSO gated)                          │
│                                                                     │
│  • Match Index sync job  ←── Bullhorn API (read-only)               │
│  • Interest queue + consultant workflow                             │
│  • Human-gated, programmatic promotion INTO Bullhorn                │
│  • Existing Mechanical Suite sourcing agents (sandbox + LinkedIn)   │
│    — UNCHANGED by this work                                         │
└─────────────────────────────────────────────────────────────────────┘
```

The public tier can be fully compromised — prompt-injected, scraped, hammered — and the worst
available outcome is disclosure of data that was already published anonymously by consenting
candidates. It has no path to Bullhorn, no path to LinkedIn, and no path to a shell.

**Chosen over the alternatives:**

| Option | Why not |
|---|---|
| Reuse Find OS sandbox agents behind a public endpoint | Gives anonymous users an agent with a computer. Non-starter. |
| Public tier queries Bullhorn directly, filters on read | One bug or injection away from exposing 100k+ CVs. Also puts unbounded public load on the customer's CRM. |
| Pure keyword search, no LLM | Loses the whole point. Tony's existing Bullhorn search *is* Boolean keyword and he explicitly named its uselessness ("24,000 results… it's almost like a bit of potluck, where do you start?"). |

## 2. The Match Index

A derived, denormalised, **pre-anonymised** store. The single most important property: identifying
fields are never written into it.

```
match_index.candidate_profile
  id                  -- public-facing "Candidate #<id>"
  bullhorn_id         -- NOT exposed to public tier (separate column-level grant)
  years_experience    -- int
  seniority           -- enum
  skills[]            -- normalised tags
  sectors[]
  region              -- "South West UK", not a postcode
  availability        -- notice period band
  salary_band
  highlights          -- 3-5 bullets, LLM-generated, de-identified
  embedding           -- vector, for semantic recall
  published           -- bool; candidate-controlled, honoured live
  consent_at, consent_text_hash
```

Two DB roles:
- `findos_sync` (internal) — read/write. Holds the Bullhorn credentials.
- `public_match_ro` (public tier) — `SELECT` on a **view** that omits `bullhorn_id` and
  `consent_*`, and hard-filters `published = true`. No write grant except `INSERT` on
  `interest_record`.

Anonymisation runs once, in the internal tier, at profile approval. The public tier never sees a
name because there is no name in its view. This is the difference between "we filter it out in the
prompt" (fragile) and "it is not in the process" (sound).

Jobs are indexed the same way from Bullhorn, filtered to the AI/ML specialism tag Leah confirmed
already exists.

**Sync job**: scheduled, internal, Bullhorn → Match Index. Read-only against Bullhorn. Refreshes job
data; candidate profiles enter only via the R1 opt-in flow, never via bulk import.

## 3. The four agents

All four are the same shape: a single LLM call (or short bounded chain) with a system prompt, a
fixed tool allowlist, structured output, a token budget and a timeout. No loops, no autonomy, no
memory across requests. They are *functions that happen to use a model*, not agents with computers.

| Agent | Trigger | Tools allowed | Writes |
|---|---|---|---|
| **Profile Builder** | Candidate uploads CV | `parse_document` | Candidate's own draft profile only |
| **Job Intake** | Client submits JD | `parse_document` | Client's own draft job spec only |
| **Matcher** | Client runs a search | `search_candidates(spec) → view` | Nothing (read-only) |
| **Reverse Matcher** | Candidate views matches | `search_jobs(profile) → view` | Nothing (read-only) |

Retrieval is hybrid: vector recall over `embedding` to get a candidate set of ~100, then the LLM
scores and explains the top N. Keep the LLM out of the recall step — it keeps cost bounded and
results reproducible.

**Scoring** mirrors how the humans actually work, which Tony described precisely: keyword/skill
match gets you the ballpark, judgement does the rest. So: a deterministic skill-overlap component
plus an LLM-judged fit component, surfaced separately so a recruiter can see which is which.
Do not invent a single opaque percentage — the 20 Jul demo showed exactly why (Luke, on the
sourcing agent's scores: "it's just made one up").

**Prompt-injection boundary.** All user content is delimited and labelled as untrusted. The system
prompt states that content inside those delimiters is data to be analysed and never instructions.
Combined with §2, injection has no target: the model cannot be talked into revealing names it was
never given, or calling tools that aren't in its allowlist.

## 4. Interest flow — the human handoff

This is the product, so it gets built properly rather than as a mailto link.

1. Client clicks "Register interest" on `Candidate #235`.
2. Public tier `INSERT`s an `interest_record` (client_id, profile_id, job_spec_id, timestamp).
   That is the public tier's only write.
3. Find OS picks it up: Slack alert into the existing Find AI channel (the Find OS Slack bot is
   already wired up — Luke added it on 27 Jul), plus a row in the new Interest queue.
4. Consultant opens the Interest in Find OS, sees the **de-anonymised** candidate and the client,
   and works it by phone, exactly as today.
5. Candidate is notified by Find OS ("a company has registered interest in your profile — your Find
   AI consultant will be in touch") without naming the company.

The client never gets the candidate's identity from the product. Find AI's fee is protected by
architecture, not by policy.

## 5. What we reuse

| Need | Existing thing | Notes |
|---|---|---|
| Hosting, TLS, custom domain | Helix web service on prod SaaS | `we-find.ai` cutover is done — see `helix/design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`. Note the `:80` domain-verification gap documented there. |
| Deploy pipeline | Helix GitHub CD, PR-per-change | Leah and Tony already ship website changes this way. |
| Internal auth | Find OS Google SSO | Already gated to `@linuxrecruit.co.uk`; Luke agreed on 20 Jul to also allow `@we-find.ai`. Needed before staff can reach the Interest queue. |
| Slack notifications | Find OS Slack bot | Already in the shared channel. |
| Bullhorn read | Find OS's existing API integration | Read-only. Stays internal-tier-only. |
| Agent memory conventions | `chris-outreach` repo pattern | `CLAUDE.md` + `memory/*.md` with frontmatter and `[[links]]`. Use this for the **internal** tier's tunable rules (scoring guidance, anonymisation rules) so Tony can edit them in English. Do **not** let the public tier read from a writable memory repo — its prompts are versioned in code. |

## 6. Phasing

Four weeks to the agreed early-September launch, with Luke away 17–21 Aug.

- **Phase 1 (week 1) — Foundations.** Match Index schema + roles; Bullhorn → jobs sync; Interest
  record + Slack alert + Find OS queue. Nothing public yet. *Demoable: jobs flowing from Bullhorn
  into the index.*
- **Phase 2 (week 2) — Rosie.** CV upload, Profile Builder, candidate review/approve/consent,
  magic-link management. *Demoable: a real anonymised profile end to end.*
- **Phase 3 (week 3) — Jim.** Client accounts (consultant-approved), Job Intake, Matcher, results UI,
  Register Interest. *Demoable: the full loop, JD in → shortlist → interest → Slack ping.*
- **Phase 4 (week 4) — Hardening + seeding.** Rate limits, spend caps, audit log, injection test
  suite, pen-test pass. Consent campaign to the AI/ML Bullhorn subset. Friendly-beta candidates and
  clients (Tony offered both at kickoff).

R3 (candidate-side job matches) is the designated cut line if week 3 slips.

## 7. Notes for whoever implements this

- **The customer's constraint is commercial, not technical.** Every design question resolves the same
  way: does this keep the consultant in the middle of the placement? If not, it's wrong, however
  clever.
- **Credits are a live customer anxiety.** Leah, 25 Jun: "I'm just really conscious that I don't want
  to obviously rinse all the credits." Public traffic on a $100/month allowance needs hard per-account
  caps and cheap-model routing for the ingest agents. Reserve the expensive model for Matcher.
- **Don't let the public tier grow a browser.** The pressure will come ("could Jim just check their
  LinkedIn?"). The answer is no; that work belongs in the Mechanical Suite where a named employee
  drives it.
- **LinkedIn account safety is the project's biggest operational risk** and it is entirely in the
  internal tier. Tony hit captchas and a lockout on 20 Jul. The agreed mitigations — late-shift runs,
  log out everywhere else, possibly a donated/dedicated account — are Mechanical Suite concerns.
  Keeping the public tier LinkedIn-free is also what stops public traffic from ever burning a
  Recruiter seat.
- **`chris-outreach` (Kodit repo id 18) is the best reference** for how a mature Helix agent workflow
  is structured — read `CLAUDE.md`, `memory/operating-principles.md`, `memory/daily-workflow.md` and
  `memory/linkedin-safety-rules.md` before touching the internal tier.
