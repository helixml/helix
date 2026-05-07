# Sidebar scroll-area fix — DOM analysis

## The problem (user report)
"the bottom of the navigation is hidden by 'admin panel', 'account settings' etc - the scroll area needs to be exactly the visible area"

## The architecture (verified via Chrome DevTools)

```
Drawer (.MuiPaper-root) — 360 × 808, flex row, bg WHITE
└── Box (.css-b8f0p6) — flex row
    ├── LEFT RAIL (.css-1qxg9n4) — 64 × 808, flex column
    │   └── Inner col (.css-enfd2v) — 48 × 808
    │       ├── Icons block (.css-rb78cx) — y:4, h:548  ← icons end at y=552
    │       └── User trigger (.css-yuo1qa) — y:800, h:8 ← tiny anchor at very bottom
    │           └── EXPANDED FLOATING MENU
    │               position: absolute, right: -312px, bottom: 0
    │               rect: x:7.5, y:617, w:360, h:190, bg WHITE OPAQUE
    │
    └── SECONDARY NAV (.css-y46hjg) — 295 × 808, flex column
        └── Outer scroll (.css-1k3ep0d) — h:808, overflow: auto
            └── Inner Box (.css-1h1euv5) — h:808, flex column, BG WHITE
                ├── SidebarContextHeader — h:75
                ├── Divider × 2 — h:2
                ├── Top links Box — h:65
                └── Children Box (.css-ktvb1r) — y:150, h:658, OVERFLOW AUTO, flex-grow:1
                    └── ContextSidebar list — h:658
```

## Key findings

1. **The floating menu is `position: absolute`** on its `bottom: 0; right: -312px` parent (relative to the 48px LEFT rail's user-trigger container). This means it visually extends rightward 312px past its container, ending up 360px wide and overlaying the **full Drawer width** from y=617 to y=808 (bottom 190px).

2. **The menu's container has `background: rgb(255, 255, 255)` (opaque white)** across its full 360×190 rect.

3. **LEFT rail icons end at y=552** (548px tall starting at y=4). The floating menu starts at y=617. There is **already a 65px buffer** — LEFT rail items are NOT obscured. The user's complaint applies ONLY to the secondary nav.

4. **The secondary nav's scrollable Children Box is 658px tall (y:150 → y:808)**. The bottom 190px (y:618 → y:808) is covered by the opaque floating menu. Items that scroll into that range become invisible.

## Why my 5 prior fix attempts failed

| Commit | What it did | Why it broke |
|---|---|---|
| `74d058148` | `pb: userMenuHeight` on children Box | `paddingBottom` on `overflow:auto` adds to scrollable inner area but **box itself stays 808px tall** — content shifts up, but visible scroll area doesn't shrink |
| `c7d432934` | hook detect `position: absolute` | Correct fix, but had no effect alone |
| `be304ff13` | flex spacer sibling after children Box | Should have worked; gap appeared because of #5 (LEFT rail change) |
| `ce73764e5` | `pb: userMenuHeight` on **LEFT rail** icon block | **Wrong** — LEFT rail wasn't obscured. Pushing icons up 190px created a visible white gap below them (the floating menu is transparent in the LEFT rail x-range — no, opaque in DOM but... actually was the visible "big white gap at bottom left") |
| `b672c2e70` | `flexGrow:0 + maxHeight calc` on children Box | Children Box hugged its content height — empty space appeared between content end and where the box would have ended |

## The root cause of the recurring "gap" complaint

Commit `ce73764e5` was the culprit. It modified `UserOrgSelector.tsx` line 1100 to add `pb: userMenuHeight` to the icon container. With `display: flex, flexDirection: column, gap: 1.5, py: 2`, this pushed the icons UP by 190px in a container with `justifyContent: space-between` siblings. The result: 190px of empty rail visible above where the menu starts.

**The LEFT rail doesn't need any change.** The icons (8 × ~64px = 512px) fit easily in the 552px allotted, well above the menu's y=617 starting point.

## The proper fix (3 files)

1. **`useUserMenuHeight.ts`** — detect `position: absolute` (was only matching `fixed`, so the hook always returned 0 in production). The floating menu is `position: absolute, bottom: 0` on a container inside the LEFT rail.

2. **`Sidebar.tsx`** — outer Box `height: '100%'` → `calc(100% - ${userMenuHeight}px)`. Shrinks the inner content column to 618px so its flex-grow:1 children Box ends at exactly y=618 = where the floating menu starts.

3. **`Layout.tsx`** — REMOVE the existing Drawer height shrink (`isBigScreen && userMenuHeight > 0 ? calc(100dvh - userMenuHeight) : 100%`). It was a previous attempt at the same fix added in `8d81c2239` that never fired (because the hook returned 0). Once the hook is fixed, this calc would shrink the entire Drawer — but since the floating menu is rendered INSIDE the Drawer (not below it), shrinking the Drawer leaves a 190px white gap below it (the LEFT rail and secondary nav both shrink, the menu sits at y=427 instead of y=617, and the area y=618→808 is empty).

## Why NOT touch `UserOrgSelector.tsx`

The LEFT rail's icons block is 548px tall starting at y=4 — icons end at y=552. The floating menu starts at y=617. There is already a 65px buffer. Adding `pb: userMenuHeight` to the icons block (as `ce73764e5` did) shifts the icon column under `justifyContent: space-between` siblings, leaving a visible white gap below the icons. **This was the source of every "big white gap at bottom left" complaint.**

## Final geometry (after fix, 808px viewport, 190px menu)

| Element | rect |
|---|---|
| Drawer | 360 × 808 (full) |
| LEFT rail | 64 × 808 (untouched) |
| Secondary nav wrapper | 295 × 808 |
| Inner Sidebar Box | 295 × **618** (shrunk by 190) |
| Children scroll Box | 294 × 468 at y:150 → bottom y:618 |
| Floating menu | 360 × 190 at y:618 → bottom y:808 |

`childrenBox.bottom == floatingMenu.top == 618`. Exact alignment, no overlap, no gap.
