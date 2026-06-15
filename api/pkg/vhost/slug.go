package vhost

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var slugSanitize = regexp.MustCompile(`[^a-z0-9-]+`)

// AllocateDefaultSubdomain returns a hostname of the form
// `<slug>.<baseDomain>`, suffixing the slug with `-2`, `-3`, … on
// collision with the reserved-hostname rules or an existing row. Returns
// the chosen hostname or an error if no free name is found within
// maxAttempts tries.
func AllocateDefaultSubdomain(ctx context.Context, projectSlug, baseDomain string, opts Options, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = 50
	}
	slug := normalizeSlug(projectSlug)
	if slug == "" {
		return "", fmt.Errorf("project slug is empty after sanitization")
	}
	if baseDomain == "" {
		return "", fmt.Errorf("base domain is required")
	}
	base := strings.ToLower(baseDomain)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		candidate := slug
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d", slug, attempt)
		}
		hostname := candidate + "." + base

		reserveOpts := opts
		reserveOpts.Hostname = hostname

		err := ReserveHostname(ctx, reserveOpts)
		if err == nil {
			return hostname, nil
		}
		if !errors.Is(err, ErrHostnameReserved) {
			return "", err
		}
		// reserved, try the next suffix
	}
	return "", fmt.Errorf("no free default subdomain found for slug %q within %d attempts", slug, maxAttempts)
}

// normalizeSlug lowercases, replaces underscores/spaces with dashes, and
// strips anything that isn't [a-z0-9-]. Collapses runs of dashes.
func normalizeSlug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = slugSanitize.ReplaceAllString(s, "")
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}

// MintShareHostname loops GenerateShareHostname + ReserveHostname until
// a free name is found or maxAttempts is exhausted. Pass an Options with
// AllowSharePrefix=true (the helper enforces this — preview minting is
// the one callsite allowed to claim a share-* name) and Store set so
// the uniqueness check runs.
func MintShareHostname(ctx context.Context, baseDomain string, opts Options, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	if opts.Store == nil {
		return "", fmt.Errorf("MintShareHostname requires a Store for uniqueness check")
	}
	opts.AllowSharePrefix = true
	for i := 0; i < maxAttempts; i++ {
		host, err := GenerateShareHostname(baseDomain)
		if err != nil {
			return "", err
		}
		opts.Hostname = host
		if err := ReserveHostname(ctx, opts); err == nil {
			return host, nil
		} else if !errors.Is(err, ErrHostnameReserved) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not mint a unique share hostname after %d attempts", maxAttempts)
}
