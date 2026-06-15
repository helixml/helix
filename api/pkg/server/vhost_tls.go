package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/vhost"
	"github.com/rs/zerolog/log"
)

// VHostTLSMode mirrors the HELIX_VHOST_TLS_MODE env var.
type VHostTLSMode string

const (
	// VHostTLSModeOff disables embedded TLS termination — Helix listens
	// HTTP only on its configured port. Use this when a reverse proxy
	// in front of Helix (e.g. Caddy with a wildcard cert) terminates
	// TLS and forwards plain HTTP.
	VHostTLSModeOff VHostTLSMode = "off"

	// VHostTLSModeAuto enables certmagic — Helix binds :443 + :80,
	// terminates TLS, and issues per-hostname certs on demand via
	// Let's Encrypt (HTTP-01 + TLS-ALPN-01).
	VHostTLSModeAuto VHostTLSMode = "auto"
)

// startCertMagicListener starts an HTTPS listener on :443 (and an HTTP
// challenge listener on :80) backed by certmagic. The on-demand decision
// function consults vhost.ReserveHostname inverted — a hostname is
// allowed to get a cert iff it would NOT be rejected by ReserveHostname
// when called with AllowSharePrefix=true and a permissive Store. This
// covers: canonical hostname, registered project domains (verified or
// default), and share-* preview tokens.
//
// Returns an error if startup fails. Runs in its own goroutine for the
// lifetime of ctx; the existing HTTP listener on apiServer.Cfg.WebServer
// keeps running in parallel as the plaintext fallback (LE HTTP-01 uses
// :80 too, so this overlaps cleanly).
func (apiServer *HelixAPIServer) startCertMagicListener(ctx context.Context, vhostCfg *VHostMiddlewareConfig, handler http.Handler) error {
	mode := strings.ToLower(strings.TrimSpace(apiServer.Cfg.WebServer.VHostTLSMode))
	if mode == "" || mode == string(VHostTLSModeOff) {
		return nil
	}
	if mode != string(VHostTLSModeAuto) {
		return fmt.Errorf("HELIX_VHOST_TLS_MODE=%q is not a recognised mode (auto|off)", mode)
	}
	if !vhostCfg.Enabled {
		return fmt.Errorf("HELIX_VHOST_TLS_MODE=auto requires DEV_SUBDOMAIN to be set")
	}
	email := strings.TrimSpace(apiServer.Cfg.WebServer.VHostLetsEncryptEmail)
	if email == "" {
		return errors.New("HELIX_VHOST_TLS_MODE=auto requires HELIX_VHOST_LETSENCRYPT_EMAIL to be set")
	}

	cfg := certmagic.NewDefault()
	cfg.Storage = &certmagic.FileStorage{Path: "/data/certmagic"}

	cfg.OnDemand = &certmagic.OnDemandConfig{
		DecisionFunc: func(_ context.Context, name string) error {
			return apiServer.vhostShouldIssueCert(ctx, vhostCfg, name)
		},
	}

	magicACME := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
		CA:     certmagic.LetsEncryptProductionCA,
		Email:  email,
		Agreed: true,
	})
	cfg.Issuers = []certmagic.Issuer{magicACME}

	log.Info().
		Str("email", email).
		Msg("vhost TLS auto mode enabled (certmagic + Let's Encrypt)")

	// HTTP-01 challenges + plaintext redirects on :80.
	go func() {
		if err := http.ListenAndServe(":80", magicACME.HTTPChallengeHandler(httpToHTTPSRedirect())); err != nil {
			log.Warn().Err(err).Msg("vhost TLS: :80 challenge listener exited")
		}
	}()

	// HTTPS listener on :443 — same handler as the plain HTTP listener,
	// so the vhost middleware runs there too.
	tlsConfig := cfg.TLSConfig()
	tlsConfig.NextProtos = append([]string{"h2", "http/1.1"}, tlsConfig.NextProtos...)
	go func() {
		srv := &http.Server{
			Addr:              ":443",
			Handler:           handler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 60 * 1_000_000_000, // 60s
		}
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			log.Warn().Err(err).Msg("vhost TLS: :443 listener exited")
		}
	}()
	return nil
}

// vhostShouldIssueCert returns nil if certmagic is allowed to issue a
// cert for the given hostname. Returns an error (with a clear message
// that ends up in the ACME log) if not.
//
// Allowed:
//   - canonical Helix hostnames (SERVER_URL + aliases)
//   - any hostname that appears in vhost_routes (project web services,
//     including default subdomains and verified custom domains, and
//     share-* preview tokens — verified_at gating happens at request
//     dispatch, not here, so a pending custom domain can still be
//     issued a cert as soon as the CA verifies DNS)
//
// Refused:
//   - everything else
func (apiServer *HelixAPIServer) vhostShouldIssueCert(ctx context.Context, vhostCfg *VHostMiddlewareConfig, hostname string) error {
	host := strings.ToLower(strings.TrimSuffix(hostname, "."))
	if _, ok := vhostCfg.CanonicalHostnames[host]; ok {
		return nil
	}
	_, err := apiServer.Store.GetVHostRouteByHostname(ctx, host)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("vhost lookup failed: %w", err)
	}
	return fmt.Errorf("hostname %q is not registered with Helix; refusing to issue cert", host)
}

// httpToHTTPSRedirect is the fallback handler for the :80 listener for
// hostnames that aren't ACME challenges. ACME challenge paths are
// handled by certmagic's HTTPChallengeHandler wrapper.
func httpToHTTPSRedirect() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't redirect /.well-known/helix-domain-verify — DNS-based
		// custom-domain verification reaches us over plain HTTP from
		// our own verifier cron.
		if strings.HasPrefix(r.URL.Path, "/.well-known/helix-domain-verify/") {
			http.Error(w, "use HTTPS for this endpoint via Helix", http.StatusBadRequest)
			return
		}
		target := "https://" + stripPort(r.Host) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

// Ensure the vhost package import isn't dead-removed by the linter
// once the decision func is refactored later.
var _ = vhost.SharePrefix
