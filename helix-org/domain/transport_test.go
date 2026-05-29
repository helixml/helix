package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTransportValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		t       Transport
		wantErr string // substring; "" means no error
	}{
		{
			name: "local no config",
			t:    Transport{Kind: TransportLocal},
		},
		{
			name: "local ignores config",
			// LocalTransport doesn't parse Config — junk should still be valid.
			t: Transport{Kind: TransportLocal, Config: json.RawMessage(`not json at all`)},
		},
		{
			name: "webhook inbound only",
			t:    Transport{Kind: TransportWebhook},
		},
		{
			name: "webhook empty config",
			t:    Transport{Kind: TransportWebhook, Config: json.RawMessage(`{}`)},
		},
		{
			name: "webhook outbound https",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"https://example.com/hook"}`),
			},
		},
		{
			name: "webhook outbound http localhost",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"http://localhost:9000"}`),
			},
		},
		{
			name:    "empty kind",
			t:       Transport{},
			wantErr: "transport kind is empty",
		},
		{
			name:    "unknown kind",
			t:       Transport{Kind: "smtp"},
			wantErr: "unknown transport kind",
		},
		{
			name: "webhook config malformed json",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{not json`),
			},
			wantErr: "parse webhook config",
		},
		{
			name: "webhook outbound non-http scheme",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"ftp://example.com/hook"}`),
			},
			wantErr: "absolute http(s) URL",
		},
		{
			name: "webhook outbound relative url",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"/just/a/path"}`),
			},
			wantErr: "absolute http(s) URL",
		},
		{
			name: "webhook outbound no host",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"http:///nohost"}`),
			},
			wantErr: "no host",
		},
		{
			name: "webhook outbound malformed url",
			t: Transport{
				Kind:   TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"http://%zz"}`),
			},
			wantErr: "outbound_url",
		},
		{
			name: "email valid alias",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"sam"}`),
			},
		},
		{
			name: "email valid alias with dash",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"customer-service"}`),
			},
		},
		{
			name:    "email missing alias",
			t:       Transport{Kind: TransportEmail},
			wantErr: "alias is required",
		},
		{
			name: "email empty alias",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":""}`),
			},
			wantErr: "alias is required",
		},
		{
			name: "email alias with @",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"sam@x"}`),
			},
			wantErr: "lowercase alphanumeric",
		},
		{
			name: "email alias with +",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"sa+m"}`),
			},
			wantErr: "lowercase alphanumeric",
		},
		{
			name: "email alias with dot",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"sam.x"}`),
			},
			wantErr: "lowercase alphanumeric",
		},
		{
			name: "email alias uppercase",
			t: Transport{
				Kind:   TransportEmail,
				Config: json.RawMessage(`{"alias":"Sam"}`),
			},
			wantErr: "lowercase alphanumeric",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.t.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %q, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestWebhookConfigParse(t *testing.T) {
	t.Parallel()

	t.Run("local rejects parse", func(t *testing.T) {
		t.Parallel()
		_, err := Transport{Kind: TransportLocal}.WebhookConfig()
		if err == nil {
			t.Fatalf("expected error parsing local transport as webhook")
		}
	})

	t.Run("empty config returns zero value", func(t *testing.T) {
		t.Parallel()
		c, err := Transport{Kind: TransportWebhook}.WebhookConfig()
		if err != nil {
			t.Fatalf("WebhookConfig() = %v, want nil", err)
		}
		if c.OutboundURL != "" {
			t.Fatalf("OutboundURL = %q, want empty", c.OutboundURL)
		}
	})

	t.Run("populated config round-trips", func(t *testing.T) {
		t.Parallel()
		raw := json.RawMessage(`{"outbound_url":"https://example.com/x"}`)
		c, err := Transport{Kind: TransportWebhook, Config: raw}.WebhookConfig()
		if err != nil {
			t.Fatalf("WebhookConfig() = %v", err)
		}
		if c.OutboundURL != "https://example.com/x" {
			t.Fatalf("OutboundURL = %q", c.OutboundURL)
		}
	})

	t.Run("unknown json fields ignored", func(t *testing.T) {
		t.Parallel()
		raw := json.RawMessage(`{"outbound_url":"https://example.com/x","future":"ignored"}`)
		c, err := Transport{Kind: TransportWebhook, Config: raw}.WebhookConfig()
		if err != nil {
			t.Fatalf("WebhookConfig() = %v", err)
		}
		if c.OutboundURL != "https://example.com/x" {
			t.Fatalf("OutboundURL = %q", c.OutboundURL)
		}
	})
}
