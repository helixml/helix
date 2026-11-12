package webservice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

func TestValidateHostname(t *testing.T) {
	for _, ok := range []string{"app.example.com", "a-b.example.co.uk", "x1.example.io"} {
		assert.NoError(t, ValidateHostname(ok), ok)
	}
	for _, bad := range []string{
		"",
		"localhost",
		"foo.localhost",
		"printer.local",
		"metadata.google.internal",
		"127.0.0.1",
		"169.254.169.254",
		"::1",
		"example.com:8080",
		"example.com/path",
		"user@example.com",
		"example.com#frag",
		"-bad.example.com",
		"bad-.example.com",
		"a..example.com",
		"Example.com",
		"example",
		"1.2.3.4444",
	} {
		assert.ErrorIs(t, ValidateHostname(bad), ErrInvalidHostname, bad)
	}
}

// The verifier must not fetch an address the policy blocks: a valid hostname
// can still resolve to loopback, an internal service or cloud metadata.
func TestDomainVerifierRefusesBlockedAddresses(t *testing.T) {
	const token = "tok123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, token)
	}))
	defer server.Close()
	target := server.URL + "/.well-known/helix-domain-verify/" + token

	blocked := NewDomainVerifier(nil, netguard.Policy{})
	assert.False(t, blocked.fetchToken(context.Background(), target, token), "loopback must be refused by default")

	allowed := NewDomainVerifier(nil, netguard.Policy{AllowPrivate: true})
	assert.True(t, allowed.fetchToken(context.Background(), target, token), "operator opt-in allows private networks")
}

func TestDomainVerifierSkipsInvalidHostnames(t *testing.T) {
	v := NewDomainVerifier(nil, netguard.Policy{AllowPrivate: true})
	assert.False(t, v.checkRow(context.Background(), &types.VHostRoute{Hostname: "127.0.0.1:8080", VerificationToken: "t"}))
}
