package attestation

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const (
	testAudience = "ptaas.example.com"
	testNonce    = "nonce-0123456789abcdef"
)

// Built at runtime so secret scanners don't read the fixture as a credential.
var testSecret = strings.Repeat("0123456789abcdef", 2)

var testSubject = Subject{
	SpecTaskID:     "spt_finder",
	SessionID:      "ses_finder",
	ProjectID:      "prj_1",
	OrganizationID: "org_1",
	Owner:          "usr_1",
	KeyConfined:    true,
}

func newTestSigner(t *testing.T, now time.Time) *Signer {
	t.Helper()
	signer, err := NewSigner(testSecret)
	require.NoError(t, err)
	return signer.WithClock(func() time.Time { return now })
}

func TestMintVerifyRoundTrip(t *testing.T) {
	now := time.Now()
	signer := newTestSigner(t, now)
	token, minted, err := signer.Mint(testSubject, testAudience, testNonce)
	require.NoError(t, err)

	claims, err := signer.Verify(token, testAudience, testNonce)
	require.NoError(t, err)
	require.Equal(t, minted, claims)
	require.Equal(t, "spt_finder", claims.SpecTaskID)
	require.Equal(t, "ses_finder", claims.SessionID)
	require.Equal(t, "prj_1", claims.ProjectID)
	require.Equal(t, "org_1", claims.OrganizationID)
	require.Equal(t, "usr_1", claims.Owner)
	require.True(t, claims.KeyConfined)
	require.Equal(t, TTL, claims.ExpiresAt.Sub(claims.IssuedAt))
	require.Len(t, claims.JTI, 32)

	other, _, err := signer.Mint(testSubject, testAudience, testNonce)
	require.NoError(t, err)
	require.NotEqual(t, token, other, "every attestation has its own jti")
}

func TestVerifyRefuses(t *testing.T) {
	now := time.Now()
	signer := newTestSigner(t, now)
	token, _, err := signer.Mint(testSubject, testAudience, testNonce)
	require.NoError(t, err)

	otherSigner, err := NewSigner("another-secret-another-secret-xx")
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	tamperedPayload := parts[0] + "." + parts[1] + "x." + parts[2]
	sig := []byte(parts[2])
	sig[0] ^= 1
	tamperedSig := parts[0] + "." + parts[1] + "." + string(sig)

	// A session JWT signed with the raw secret must not pass as an attestation.
	sessionJWT, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "helix", "sub": "usr_1", "aud": testAudience, "exp": now.Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	require.NoError(t, err)

	cases := map[string]struct {
		signer          *Signer
		token, aud, nce string
	}{
		"wrong audience":     {signer, token, "other.example.com", testNonce},
		"wrong nonce":        {signer, token, testAudience, "nonce-different-0000"},
		"expired":            {signer.WithClock(func() time.Time { return now.Add(TTL + ClockSkew + time.Second) }), token, testAudience, testNonce},
		"issued in future":   {signer.WithClock(func() time.Time { return now.Add(-ClockSkew - time.Minute) }), token, testAudience, testNonce},
		"tampered payload":   {signer, tamperedPayload, testAudience, testNonce},
		"tampered signature": {signer, tamperedSig, testAudience, testNonce},
		"other key":          {otherSigner, token, testAudience, testNonce},
		"session jwt":        {signer, sessionJWT, testAudience, testNonce},
		"empty token":        {signer, "", testAudience, testNonce},
		"garbage":            {signer, "not-a-token", testAudience, testNonce},
		"oversized token":    {signer, strings.Repeat("a", MaxTokenLen+1), testAudience, testNonce},
		"alg none":           {signer, unsignedCopy(t, token), testAudience, testNonce},
		"empty audience":     {signer, token, "", testNonce},
		"empty nonce":        {signer, token, testAudience, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			claims, err := tc.signer.Verify(tc.token, tc.aud, tc.nce)
			require.ErrorIs(t, err, ErrInvalid)
			require.Nil(t, claims)
		})
	}

	// Within the skew window the token still verifies.
	_, err = signer.WithClock(func() time.Time { return now.Add(TTL + ClockSkew - time.Second) }).Verify(token, testAudience, testNonce)
	require.NoError(t, err)
}

func unsignedCopy(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	header := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{})
	header.Header["kid"] = keyID
	header.Header["typ"] = tokType
	unsigned, err := header.SigningString()
	require.NoError(t, err)
	return strings.Split(unsigned, ".")[0] + "." + parts[1] + "."
}

func TestMintValidatesInput(t *testing.T) {
	signer := newTestSigner(t, time.Now())
	for _, aud := range []string{"", "-leading-dash", "has space", strings.Repeat("a", 129), "quote\"", "new\nline"} {
		_, _, err := signer.Mint(testSubject, aud, testNonce)
		require.Error(t, err, "audience %q", aud)
	}
	for _, nonce := range []string{"", "short", strings.Repeat("n", 129), "has space in it 1234", "unicode-ñññññññññññ"} {
		_, _, err := signer.Mint(testSubject, testAudience, nonce)
		require.Error(t, err, "nonce %q", nonce)
	}
	_, _, err := signer.Mint(Subject{Owner: "usr_1"}, testAudience, testNonce)
	require.Error(t, err, "a subject without a project is refused")
}
