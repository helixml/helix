# Clone Feature Demo: Brand Color

The clone feature lets you apply learnings from one task to many similar repositories. This demo shows how it works.

## The Setup

Five projects, each containing a different shape:
- Circle (Start Here)
- Square
- Triangle
- Hexagon
- Star

Each shape is white with a black outline. The task: "Fill the shape with the company's brand color."

## The Trick

The brand color isn't specified anywhere in the code. The agent *must* ask the user.

This is the key insight: the spec says "fill with brand color" but the implementation requires discovering what that color actually is. The agent learns this by asking the user.

## Why This Matters

Yes, picking a color is trivial. It's a classic bikeshedding example. But use your imagination.

**The color is a stand-in for any hard-to-discover fact:**
- The correct API endpoint that isn't documented
- The database migration strategy the team decided on last week
- The authentication pattern used elsewhere in the codebase
- The edge case that requires special handling

In real projects, discovering these facts takes time. The agent reads code, tries approaches, hits dead ends, asks clarifying questions. This might take 30 minutes on the first repository.

## The Clone Payoff

Once the agent discovers the brand color (or database strategy, or API pattern), that learning is captured in the spec:

```markdown
## Implementation Notes

Brand color confirmed with user: #3B82F6 (blue)
```

When you clone the task to the other four shapes, the spec already contains this knowledge. The agent doesn't need to ask again. It just applies the color.

**First task:** 30 minutes (discovery + implementation)
**Four cloned tasks:** 1 minute each (implementation only)

Total: ~35 minutes for 5 repositories instead of 2.5 hours.

## Running the Demo

1. Fork the "Brand Color Demo" sample project
2. Five shape projects are created
3. Start the task on "Circle (Start Here)"
4. Agent asks: "What is your company's brand color?"
5. You answer: "Blue" (or #3B82F6, or "surprise me with a pig")
6. Agent fills the circle, updates the spec
7. Click "Clone" on the completed task
8. Select the other four shape projects
9. Watch all shapes fill with the same color

The viewer pages auto-refresh, so you can see the changes live across all desktops.

## Technical Notes

- Clone now uses the latest spec from `SpecTaskDesignReview` (not the original stale spec)
- Each shape project has a `viewer.html` that refreshes every second
- The startup script opens the viewer automatically
