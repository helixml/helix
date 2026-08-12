package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/config"
)

func TestACMEChallengeTarget(t *testing.T) {
	cases := []struct {
		name        string
		provider    string
		override    string
		cnameTarget string
		want        string
	}{
		{
			name:        "derived from registrable domain when cloudflare enabled",
			provider:    "cloudflare",
			cnameTarget: "ingress.helix.ml",
			want:        "_acme-challenge.helix.ml",
		},
		{
			name:        "case-insensitive provider match",
			provider:    "Cloudflare",
			cnameTarget: "ingress.helix.ml",
			want:        "_acme-challenge.helix.ml",
		},
		{
			name:        "explicit override wins over derivation",
			provider:    "cloudflare",
			override:    "_acme-challenge.custom-zone.example",
			cnameTarget: "ingress.helix.ml",
			want:        "_acme-challenge.custom-zone.example",
		},
		{
			name:        "multi-level public suffix",
			provider:    "cloudflare",
			cnameTarget: "ingress.helix.co.uk",
			want:        "_acme-challenge.helix.co.uk",
		},
		{
			name:        "trailing dot on cname target is tolerated",
			provider:    "cloudflare",
			cnameTarget: "ingress.helix.ml.",
			want:        "_acme-challenge.helix.ml",
		},
		{
			name:        "hidden when DNS-01 provider not cloudflare",
			provider:    "",
			cnameTarget: "ingress.helix.ml",
			want:        "",
		},
		{
			name:        "override ignored when provider not cloudflare",
			provider:    "",
			override:    "_acme-challenge.helix.ml",
			cnameTarget: "ingress.helix.ml",
			want:        "",
		},
		{
			name:        "no registrable domain (localhost) yields empty",
			provider:    "cloudflare",
			cnameTarget: "localhost",
			want:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &HelixAPIServer{
				Cfg: &config.ServerConfig{
					WebServer: config.WebServer{
						VHostACMEDNSProvider:     tc.provider,
						VHostACMEChallengeTarget: tc.override,
					},
				},
			}
			if got := s.acmeChallengeTarget(tc.cnameTarget); got != tc.want {
				t.Errorf("acmeChallengeTarget(%q) = %q, want %q", tc.cnameTarget, got, tc.want)
			}
		})
	}
}
