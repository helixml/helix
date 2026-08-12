package vhost

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGenerateShareHostnameStructure(t *testing.T) {
	got, err := GenerateShareHostname("dev.helix.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, SharePrefix) {
		t.Errorf("expected share- prefix, got %q", got)
	}
	if !strings.HasSuffix(got, ".dev.helix.example.com") {
		t.Errorf("expected base domain suffix, got %q", got)
	}
	// Expect: share-<adj>-<noun>-<8hex>.<base>
	label := strings.TrimSuffix(got, ".dev.helix.example.com")
	parts := strings.Split(strings.TrimPrefix(label, SharePrefix), "-")
	if len(parts) != 3 {
		t.Errorf("expected 3 dash-separated segments after %q, got %d (%q)", SharePrefix, len(parts), label)
	}
	if len(parts) >= 3 && len(parts[2]) != 8 {
		t.Errorf("expected 8-char hex suffix, got %q", parts[2])
	}
}

func TestGenerateShareHostnameUniqueness(t *testing.T) {
	const N = 5000
	seen := make(map[string]struct{}, N)
	for i := 0; i < N; i++ {
		h, err := GenerateShareHostname("base.example.com")
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if _, dup := seen[h]; dup {
			t.Fatalf("duplicate generated after %d attempts: %q", i, h)
		}
		seen[h] = struct{}{}
	}
}

func TestIsShareHostname(t *testing.T) {
	cases := map[string]bool{
		"share-purple-otter-abcd1234.dev.example.com": true,
		"SHARE-X-Y-12345678.dev.example.com":          true, // case-insensitive
		"app.dev.example.com":                         false,
		"":                                            false,
	}
	for in, want := range cases {
		if got := IsShareHostname(in); got != want {
			t.Errorf("IsShareHostname(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestReserveHostnameRejections(t *testing.T) {
	opts := Options{
		CanonicalServerURL:  "https://helix.example.com",
		CanonicalAliases:    []string{"app-internal.example.com"},
		BaseDomain:          "dev.helix.example.com",
		ExtraReservedLabels: []string{"intranet"},
	}

	cases := []struct {
		name string
		host string
		want bool // true = should be rejected
	}{
		{"canonical hostname", "helix.example.com", true},
		{"canonical alias", "app-internal.example.com", true},
		{"base apex", "dev.helix.example.com", true},
		{"reserved label api", "api.dev.helix.example.com", true},
		{"reserved label www", "www.dev.helix.example.com", true},
		{"operator reserved label", "intranet.dev.helix.example.com", true},
		{"share-prefix label", "share-purple-otter-abcd1234.dev.helix.example.com", true},
		{"normal slug", "myproject.dev.helix.example.com", false},
		{"unrelated domain", "example.com", false},
		{"sub-of-reserved", "foo.api.dev.helix.example.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := opts
			o.Hostname = tc.host
			err := ReserveHostname(context.Background(), o)
			rejected := errors.Is(err, ErrHostnameReserved)
			if rejected != tc.want {
				t.Errorf("ReserveHostname(%q) rejected=%v want=%v (err=%v)", tc.host, rejected, tc.want, err)
			}
		})
	}
}

func TestReserveHostnameAllowSharePrefix(t *testing.T) {
	opts := Options{
		BaseDomain:       "dev.helix.example.com",
		Hostname:         "share-purple-otter-abcd1234.dev.helix.example.com",
		AllowSharePrefix: true,
	}
	if err := ReserveHostname(context.Background(), opts); err != nil {
		t.Errorf("share-prefix should be allowed when AllowSharePrefix=true; got error: %v", err)
	}
}

func TestNormalizeSlug(t *testing.T) {
	cases := map[string]string{
		"MyProject":      "myproject",
		"my project":     "my-project",
		"my_project":     "my-project",
		"foo!@#$bar":     "foobar",
		"a---b":          "a-b",
		"-leading-dash-": "leading-dash",
	}
	for in, want := range cases {
		if got := normalizeSlug(in); got != want {
			t.Errorf("normalizeSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
