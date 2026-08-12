package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

// buildACMEChallengeSolver returns the DNS-01 solver and a human-
// readable description of the challenge type that certmagic will use.
//
// A nil solver means: leave the ACMEIssuer.DNS01Solver field unset so
// certmagic falls back to HTTP-01 + TLS-ALPN-01 (the existing
// behaviour). A non-nil solver disables those network challenges (per
// certmagic docs) and uses DNS-01 exclusively.
//
// Returns an error for unsupported provider values or missing
// credentials so startup fails loudly rather than at first cert issue.
func buildACMEChallengeSolver(ws config.WebServer) (*certmagic.DNS01Solver, string, error) {
	provider := strings.ToLower(strings.TrimSpace(ws.VHostACMEDNSProvider))
	switch provider {
	case "":
		if strings.TrimSpace(ws.VHostCloudflareAPIToken) != "" {
			log.Warn().Msg("HELIX_VHOST_CLOUDFLARE_API_TOKEN is set but HELIX_VHOST_ACME_DNS_PROVIDER is not — ignoring token, falling back to HTTP-01 + TLS-ALPN-01")
		}
		return nil, "http-01 + tls-alpn-01", nil
	case "cloudflare":
		token := strings.TrimSpace(ws.VHostCloudflareAPIToken)
		if token == "" {
			return nil, "", errors.New("HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare requires HELIX_VHOST_CLOUDFLARE_API_TOKEN")
		}
		solver := &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &cloudflare.Provider{APIToken: token},
				// Public resolvers so _acme-challenge CNAME delegation
				// resolves reliably: a custom domain not in our zone can
				// CNAME _acme-challenge.<host> into our Cloudflare zone, and
				// certmagic follows that CNAME to place the TXT where our
				// token has access. The container's default resolver may be
				// internal and not see those external records.
				Resolvers: []string{"1.1.1.1:53", "8.8.8.8:53"},
			},
		}
		return solver, "dns-01 via cloudflare (with CNAME delegation)", nil
	default:
		return nil, "", fmt.Errorf("HELIX_VHOST_ACME_DNS_PROVIDER=%q is not supported (supported: cloudflare)", provider)
	}
}
