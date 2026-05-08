package license

import (
	"encoding/hex"
	"testing"
)

// TestRevokedDenylistPinned fails if the denylist is empty. Entries
// here are revocations that have shipped; silently deleting them
// resurrects licenses that were intentionally rejected.
func TestRevokedDenylistPinned(t *testing.T) {
	if len(revokedLicenseIDHashes) == 0 {
		t.Fatal("revokedLicenseIDHashes must not be empty; removing entries undoes past revocations")
	}
}

// TestRevokedHashFormat verifies every key is a valid hex-encoded
// SHA-256 hash. A malformed key would silently never match at runtime.
func TestRevokedHashFormat(t *testing.T) {
	for h, code := range revokedLicenseIDHashes {
		if len(h) != 64 {
			t.Fatalf("hash %q has length %d, expected 64 hex chars", h, len(h))
		}
		if _, err := hex.DecodeString(h); err != nil {
			t.Fatalf("hash %q is not valid hex: %v", h, err)
		}
		if code == "" {
			t.Fatalf("hash %q has empty code", h)
		}
	}
}

// TestIsRevokedHashesInput confirms isRevoked looks up by SHA-256 of
// the license ID rather than by plaintext. An unknown ID returns
// false; a plaintext that happens to match a hash key does not match.
func TestIsRevokedHashesInput(t *testing.T) {
	if _, revoked := isRevoked("lic_never_issued"); revoked {
		t.Fatal("unknown license ID must not be reported as revoked")
	}

	for h := range revokedLicenseIDHashes {
		// Feeding the hash itself as the license ID must not match,
		// because isRevoked must hash its input before lookup.
		if _, revoked := isRevoked(h); revoked {
			t.Fatal("isRevoked matched on raw key; it must hash its input first")
		}
	}
}
