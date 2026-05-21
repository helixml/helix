import React, { useCallback, useRef, useState } from 'react'
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'

import InlineCommentForm from './InlineCommentForm'

// Regression: in 41052d44b the bubble-stacking fix wired an `outerRef`
// from DesignReviewContent (parent) into InlineCommentForm (child) so the
// form could participate in collision math. The parent invoked `outerRef`
// by bumping a `commentFormMeasureTick` state. The child, however, merged
// its local paperRef with `outerRef` via an INLINE function, giving the
// merged ref callback a fresh identity on every render. React responds to
// a changed ref callback by invoking the old one with (null) then the new
// one with (node) — each call ran outerRef(el), which bumped parent state,
// which re-rendered the child, which produced yet another fresh inline
// merge function. Infinite loop, "Maximum update depth exceeded" in Sentry.
//
// This test mounts the form inside a parent that mimics the exact pattern.
// If the merged ref callback ever becomes identity-unstable again, React
// will throw during render and the test fails.

function MeasureTickParent() {
  const [, setMeasureTick] = useState(0)
  const formHeightRef = useRef<number>(0)

  // Stable outerRef — same shape as handleCommentFormRef in
  // DesignReviewContent.tsx. The bug is reproducible even with this
  // stable parent callback because the regression lives on the child side.
  const handleOuterRef = useCallback((el: HTMLDivElement | null) => {
    formHeightRef.current = el?.offsetHeight ?? 0
    setMeasureTick((t) => t + 1)
  }, [])

  return (
    <InlineCommentForm
      show={true}
      yPos={100}
      selectedText="some highlighted text"
      commentText=""
      onCommentChange={() => {}}
      onCreate={() => {}}
      onCancel={() => {}}
      outerRef={handleOuterRef}
    />
  )
}

describe('InlineCommentForm ref-callback stability', () => {
  it('mounts without triggering an infinite update loop when the parent bumps state from outerRef', () => {
    // React's "Maximum update depth exceeded" surfaces as a thrown error
    // from render. If the merged ref callback regresses to inline (fresh
    // identity per render), this render() call throws.
    expect(() => render(<MeasureTickParent />)).not.toThrow()
  })
})
