package controller

import (
	"strings"
	"testing"
)

// H6/H9 regression (2026-08-16 sandbox security analysis): user-supplied
// filestore paths containing ".." resolved outside the caller's prefix and
// exposed every tenant's filestore. joinUnderBase must refuse those.
func TestJoinUnderBase(t *testing.T) {
	base := "/dev/users/usr_123"
	cases := []struct {
		name    string
		rel     string
		want    string
		wantErr bool
	}{
		{"empty returns base", "", base, false},
		{"plain relative", "documents", "/dev/users/usr_123/documents", false},
		{"nested relative", "documents/a.pdf", "/dev/users/usr_123/documents/a.pdf", false},
		{"dot segment is harmless", "./documents", "/dev/users/usr_123/documents", false},
		{"leading slash is stripped", "/documents", "/dev/users/usr_123/documents", false},
		{"absolute path re-roots under base", "/etc/passwd", "/dev/users/usr_123/etc/passwd", false},
		{"parent of base", "..", "", true},
		{"sibling tenant", "../usr_456", "", true},
		{"deep sibling tenant", "../usr_456/data/file.bin", "", true},
		{"re-anchored traversal", "documents/../../../usr_456", "", true},
		{"interior parent segment", "a/../b", "", true},
		{"nul byte", "a\x00b", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := joinUnderBase(base, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("joinUnderBase(%q) = %q, want error", tc.rel, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("joinUnderBase(%q) error: %v", tc.rel, err)
			}
			if got != tc.want {
				t.Fatalf("joinUnderBase(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}

	// Whatever survives validation can never escape the base prefix.
	contained, err := joinUnderBase(base, "documents/x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(contained, base+"/") {
		t.Fatalf("path escaped base: %q", contained)
	}
}

func TestExtractAppID(t *testing.T) {
	id, err := ExtractAppID("apps/app_123/files/a.pdf")
	if err != nil || id != "app_123" {
		t.Fatalf("ExtractAppID = %q, %v; want app_123", id, err)
	}

	for _, p := range []string{"apps/../../users", "apps/./x", "apps/..", "notanapppath"} {
		if _, err := ExtractAppID(p); err == nil {
			t.Fatalf("ExtractAppID(%q) succeeded, want error", p)
		}
	}
}
