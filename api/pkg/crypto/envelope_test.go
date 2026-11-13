package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAES256GCMEnvelope(t *testing.T) {
	key := make([]byte, 32)
	for _, plaintext := range []string{"", "x", strings.Repeat("y", 500)} {
		enc, err := EncryptAES256GCM([]byte(plaintext), key)
		require.NoError(t, err)
		require.True(t, IsAES256GCMEnvelope(enc), "ciphertext of %d bytes", len(plaintext))
	}
	for _, legacy := range []string{
		"",
		"sk-ant-api03-abc_def",            // not base64
		"ghp_0123456789abcdefghijklmnop",  // not base64
		"aGVsbG8=",                        // base64, too short to be nonce+tag
		"https://hooks.example.com/x?y=z", // not base64
	} {
		require.False(t, IsAES256GCMEnvelope(legacy), legacy)
	}
}
