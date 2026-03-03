# Design: Update Helix License Terms

## Context

The current license (LICENSE.md) explicitly grants free use to small businesses (<250 employees, <$10M revenue) without qualification. The README reinforces this with explicit "free" language. HelixML wants to clarify these are discretionary terms that may change.

## Key Decisions

### Approach: Subtle Clarification, Not Removal

Rather than removing small business terms entirely, we'll:
1. Add "subject to change" / "at Helix's discretion" qualifiers
2. Remove explicit "free" promises from README
3. Direct users to pricing page for authoritative current terms

This avoids alienating existing users while establishing flexibility for future policy changes.

### License Changes (Section 4.2)

**Current wording issue:** Section 4.2(a) states the Personal Offering "is further restricted to" several use cases including Small Business Environment—implying entitlement.

**Proposed change:** Add qualifier that these terms "may be modified or discontinued at Helix's sole discretion" and that users should verify current eligibility on the pricing page.

### README Changes

**Current:** Explicitly lists "Small Business Use" as free with specific thresholds.

**Proposed:** Remove specific thresholds and "free" language. Replace with direction to check current licensing terms on pricing page.

## Files to Modify

| File | Change |
|------|--------|
| `LICENSE.md` | Add discretion clause to Section 4.2, update date |
| `README.md` | Reword License section (~10 lines) |

## Risks

- **User confusion:** Mitigated by keeping small business mention but clarifying it's subject to change
- **Perceived as hostile:** Mitigated by using neutral, professional language without removing benefits entirely