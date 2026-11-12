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

// Every pending custom domain must publish the token in DNS as well as echo
// it over HTTP: the echo alone is also satisfied by a previous holder's
// stale CNAME, so it proves reachability, not ownership.
func TestDomainVerifierRequiresEchoAndTXTProof(t *testing.T) {
	const token = "tok456"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/helix-domain-verify/"+token {
			fmt.Fprint(w, token)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	target := server.URL + "/.well-known/helix-domain-verify/" + token
	row := &types.VHostRoute{Hostname: "app.customer.example", VerificationToken: token}

	v := NewDomainVerifier(nil, netguard.Policy{AllowPrivate: true})
	var asked []string
	v.lookupTXT = func(_ context.Context, name string) ([]string, error) {
		asked = append(asked, name)
		return []string{"v=spf1 -all", "other"}, nil
	}
	assert.False(t, v.verifyRow(context.Background(), row, target), "HTTP echo alone must not verify a claim")
	assert.Equal(t, []string{"_helix-challenge.app.customer.example"}, asked)

	v.lookupTXT = func(context.Context, string) ([]string, error) { return nil, fmt.Errorf("NXDOMAIN") }
	assert.False(t, v.verifyRow(context.Background(), row, target))

	v.lookupTXT = func(context.Context, string) ([]string, error) { return []string{" " + token + " "}, nil }
	assert.True(t, v.verifyRow(context.Background(), row, target))

	// The TXT record alone is not enough either: the hostname must reach
	// this deployment.
	assert.False(t, v.verifyRow(context.Background(), row, server.URL+"/.well-known/helix-domain-verify/other"))

	// A different token in DNS is a different claim.
	v.lookupTXT = func(context.Context, string) ([]string, error) { return []string{"tok999"}, nil }
	assert.False(t, v.verifyRow(context.Background(), row, target))
}
