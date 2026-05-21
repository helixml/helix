# Architecture Decision Records

Each file in this directory records one architectural or design
decision the helix-org redesign has committed to. ADRs are short,
numbered sequentially, and **immutable once accepted** — if a later
decision overturns an earlier one, write a new ADR whose `Status:` is
`Accepted` and mark the old one `Superseded by ADR-NNNN`. The
history of how the system got to its current shape is preserved by
the chain.

## Format

Every ADR uses this skeleton (Michael Nygard, 2011):

```markdown
# ADR-NNNN — Short title

## Status
Proposed | Accepted | Superseded by ADR-MMMM | Deprecated

## Context
What's the situation? What problem are we solving? What constraints
matter?

## Decision
What we are going to do. Present tense, stated plainly.

## Consequences
What this buys us; what it costs. Positive and negative. What
becomes easier; what becomes harder; what's now out of scope.

## Alternatives considered (optional)
What we did not pick, and why.

## References (optional)
Links to discussion, prior ADRs, design docs.
```

## Index

| #    | Title | Status |
|------|-------|--------|
| 0001 | [Terminology pinned for the helix-org redesign](0001-terminology.md) | Accepted |

## How this fits the redesign

The full analysis sits under [`../2026-05-21-redesign/`](../2026-05-21-redesign/)
(eight + one docs, ~3800 lines). That analysis describes *what is*
and *what should change*. ADRs in this directory pin the *decisions*
the analysis recommends, one per file. The migration plan in
[`../2026-05-21-redesign/08-migration-plan.md`](../2026-05-21-redesign/08-migration-plan.md)
§C lists the ADRs each migration depends on.
