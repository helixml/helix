// Package yellowdog is a small Go client for the YellowDog Platform REST API.
//
// It is scoped to the endpoints Helix needs to act as a YellowDog compute
// provider: worker pools, work requirements, compute requirements,
// namespaces, and the object-store data-client paths. The full
// YellowDog SDK surface (provisioning templates, allowances, keyrings,
// image families, etc.) is intentionally NOT modelled here - those are
// operator-side concerns configured once outside of Helix's runtime.
//
// Design rationale: see
//   ~/Documents/titan-obsidian/titan/helix/design/2026-06-04-yd-go-client-decision.md
//
// The Go client is hand-written rather than codegenerated, because (a)
// YellowDog does not publish a public OpenAPI spec, (b) the surface
// Helix needs is small (10-15 endpoints), and (c) auth + JSON
// conventions are simple enough that the Java/Python SDKs' machinery
// is overkill.
package yellowdog
