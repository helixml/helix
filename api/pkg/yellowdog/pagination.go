package yellowdog

// Page wraps a single page of results from a YellowDog list endpoint.
// The platform paginates with an opaque cursor in NextSliceID. When
// the field is empty (or absent / null in the JSON), there are no
// more pages.
//
// Methods that return *Page[T] do NOT auto-paginate; callers iterate
// explicitly. This keeps memory bounded and makes failure modes
// (transient errors mid-iteration) surface to the caller.
type Page[T any] struct {
	Items       []T    `json:"items"`
	NextSliceID string `json:"nextSliceId,omitempty"`
}

// HasMore reports whether there is at least one more page after this one.
func (p *Page[T]) HasMore() bool {
	return p.NextSliceID != ""
}
