package crypto

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsingDefaultEncryptionKey(t *testing.T) {
	cases := map[string]bool{
		"":                             true,
		"helix-default-encryption-key": true, // derives the same key as unset
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": false,
		"some-operator-passphrase": false,
	}
	for value, want := range cases {
		t.Setenv("HELIX_ENCRYPTION_KEY", value)
		require.Equal(t, want, UsingDefaultEncryptionKey(), "HELIX_ENCRYPTION_KEY=%q", value)
	}
}

// The default key must stay derivable so installs that never set
// HELIX_ENCRYPTION_KEY can still decrypt their data.
func TestDefaultEncryptionKeyUnchanged(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "")
	key, err := GetEncryptionKey()
	require.NoError(t, err)
	want := sha256.Sum256([]byte("helix-default-encryption-key"))
	require.Equal(t, want[:], key)
}
