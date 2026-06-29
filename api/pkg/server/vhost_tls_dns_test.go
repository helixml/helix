package server

import (
	"strings"
	"testing"

	"github.com/libdns/cloudflare"

	"github.com/helixml/helix/api/pkg/config"
)

func TestBuildACMEChallengeSolver(t *testing.T) {
	cases := []struct {
		name        string
		provider    string
		token       string
		wantSolver  bool
		wantDesc    string
		wantErrSub  string // substring of error message; "" means no error
	}{
		{
			name:       "empty provider falls back to HTTP-01",
			wantSolver: false,
			wantDesc:   "http-01 + tls-alpn-01",
		},
		{
			name:       "empty provider with stray token still falls back",
			token:      "stray-token",
			wantSolver: false,
			wantDesc:   "http-01 + tls-alpn-01",
		},
		{
			name:       "cloudflare with token returns DNS-01 solver",
			provider:   "cloudflare",
			token:      "topsecret",
			wantSolver: true,
			wantDesc:   "dns-01 via cloudflare (with CNAME delegation)",
		},
		{
			name:       "cloudflare provider name is case-insensitive",
			provider:   "Cloudflare",
			token:      "topsecret",
			wantSolver: true,
			wantDesc:   "dns-01 via cloudflare (with CNAME delegation)",
		},
		{
			name:       "cloudflare without token errors",
			provider:   "cloudflare",
			wantErrSub: "HELIX_VHOST_CLOUDFLARE_API_TOKEN",
		},
		{
			name:       "cloudflare with whitespace-only token errors",
			provider:   "cloudflare",
			token:      "   ",
			wantErrSub: "HELIX_VHOST_CLOUDFLARE_API_TOKEN",
		},
		{
			name:       "unsupported provider errors and lists supported",
			provider:   "route53",
			token:      "irrelevant",
			wantErrSub: "not supported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := config.WebServer{
				VHostACMEDNSProvider:    tc.provider,
				VHostCloudflareAPIToken: tc.token,
			}
			solver, desc, err := buildACMEChallengeSolver(ws)

			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if desc != tc.wantDesc {
				t.Errorf("description = %q want %q", desc, tc.wantDesc)
			}
			if tc.wantSolver {
				if solver == nil {
					t.Fatalf("expected non-nil DNS01Solver")
				}
				prov, ok := solver.DNSManager.DNSProvider.(*cloudflare.Provider)
				if !ok {
					t.Fatalf("DNSProvider is %T, want *cloudflare.Provider", solver.DNSManager.DNSProvider)
				}
				if prov.APIToken != strings.TrimSpace(tc.token) {
					t.Errorf("APIToken = %q want %q", prov.APIToken, strings.TrimSpace(tc.token))
				}
				if len(solver.DNSManager.Resolvers) == 0 {
					t.Errorf("expected DNSManager.Resolvers to be set (for reliable _acme-challenge CNAME delegation)")
				}
			} else if solver != nil {
				t.Errorf("expected nil solver, got %+v", solver)
			}
		})
	}
}
