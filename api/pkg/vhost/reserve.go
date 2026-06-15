package vhost

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
)

// builtInReservedLabels is the set of single-label subdomains under the
// vhost base that no user may ever claim. They protect operational and
// well-known service names from being shadowed by a project default
// subdomain or a custom-domain registration.
var builtInReservedLabels = map[string]struct{}{
	"api":         {},
	"app":         {},
	"www":         {},
	"auth":        {},
	"admin":       {},
	"helix":       {},
	"console":     {},
	"dashboard":   {},
	"helix-admin": {},
	"mail":        {},
	"ns":          {},
}

// ErrHostnameReserved is returned by ReserveHostname when the requested
// hostname is in the protected set. The caller is responsible for
// translating this into the appropriate HTTP status (typically 409).
var ErrHostnameReserved = errors.New("hostname is reserved")

// Options carries everything ReserveHostname needs to decide. All fields
// are optional except Hostname.
type Options struct {
	// Hostname is the candidate hostname to reserve. Case-insensitive.
	Hostname string

	// CanonicalServerURL is the value of the SERVER_URL config var.
	// The hostname portion is parsed out and added to the reserved set.
	CanonicalServerURL string

	// CanonicalAliases lists additional canonical hostnames that resolve
	// to the main Helix app (e.g. internal aliases). All are reserved.
	CanonicalAliases []string

	// BaseDomain is the DEV_SUBDOMAIN-derived base (e.g.
	// "dev.helix.example.com"). The apex itself is reserved, and any
	// label under it is checked against the reserved-label set.
	BaseDomain string

	// ExtraReservedLabels are operator-configured additional reserved
	// single-label subdomains under BaseDomain
	// (HELIX_VHOST_RESERVED_SUBDOMAINS).
	ExtraReservedLabels []string

	// AllowSharePrefix lets callers that are themselves minting share-*
	// preview hostnames bypass the share-prefix block. Default false:
	// projects and custom-domain registrations can never claim a
	// share-prefixed name.
	AllowSharePrefix bool

	// Store is consulted to refuse hostnames already present in
	// vhost_routes. Pass the same Store the rest of the server uses.
	// May be nil for pure-policy callers (e.g. dry-run validation).
	Store store.Store
}

// ReserveHostname returns nil if the hostname is free to claim, an error
// wrapping ErrHostnameReserved if the hostname is in any reserved set,
// or any error returned by the store lookup. The returned error message
// is suitable for direct display to end users.
func ReserveHostname(ctx context.Context, opts Options) error {
	host := normalize(opts.Hostname)
	if host == "" {
		return fmt.Errorf("hostname is required")
	}

	canonical := normalize(extractHost(opts.CanonicalServerURL))
	if canonical != "" && host == canonical {
		return fmt.Errorf("%w: %q is the canonical Helix hostname", ErrHostnameReserved, host)
	}
	for _, alias := range opts.CanonicalAliases {
		if normalize(alias) == host {
			return fmt.Errorf("%w: %q is a canonical Helix alias", ErrHostnameReserved, host)
		}
	}

	base := normalize(opts.BaseDomain)
	if base != "" && host == base {
		return fmt.Errorf("%w: %q is the vhost base domain apex", ErrHostnameReserved, host)
	}

	if base != "" && strings.HasSuffix(host, "."+base) {
		label := strings.TrimSuffix(host, "."+base)
		// Walk every label in the subdomain segment. If any matches a
		// reserved label, reject — this prevents a user from claiming
		// `foo.api.<base>` to shadow the reserved `api.<base>` path.
		for _, part := range strings.Split(label, ".") {
			if _, isReserved := builtInReservedLabels[part]; isReserved {
				return fmt.Errorf("%w: %q contains reserved label %q", ErrHostnameReserved, host, part)
			}
			for _, extra := range opts.ExtraReservedLabels {
				if normalize(extra) == part {
					return fmt.Errorf("%w: %q contains operator-reserved label %q", ErrHostnameReserved, host, part)
				}
			}
		}
		// Leftmost label may not start with the share- prefix unless the
		// caller is the minter itself.
		leftmost := label
		if i := strings.Index(label, "."); i >= 0 {
			leftmost = label[:i]
		}
		if !opts.AllowSharePrefix && strings.HasPrefix(leftmost, SharePrefix) {
			return fmt.Errorf("%w: %q-prefixed hostnames are reserved for preview tokens", ErrHostnameReserved, SharePrefix)
		}
	}

	if opts.Store != nil {
		existing, err := opts.Store.GetVHostRouteByHostname(ctx, host)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("store lookup: %w", err)
		}
		if existing != nil {
			return fmt.Errorf("%w: %q is already registered", ErrHostnameReserved, host)
		}
	}

	return nil
}

// normalize lowercases the hostname and strips a trailing dot if present.
func normalize(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimSuffix(h, ".")
	return h
}

// extractHost pulls the hostname out of a URL string. Accepts bare hosts
// too. Returns "" if the input doesn't parse.
func extractHost(raw string) string {
	if raw == "" {
		return ""
	}
	// Try as URL first.
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	// Bare host (maybe with :port).
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		return raw[:i]
	}
	return raw
}
