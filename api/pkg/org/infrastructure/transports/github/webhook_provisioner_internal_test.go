package github

// Internal tests for the webhook provisioner. They cover the parts that
// run WITHOUT touching the real GitHub API: the pure helpers (splitRepo,
// payloadURL, resolvePublicURL, ensureWebhookSecret, resolveToken) and the
// precondition / degradation branches of Install and Status that return
// before the github client is built. The actual UpsertWebhook / FindWebhook
// round-trips need a live GitHub and are out of scope here.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

const provOrg = "org-test"

// newReg builds a config registry over the in-memory store with the two
// keys the provisioner reads/writes registered.
func newReg(t *testing.T) *configregistry.Registry {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{Key: "transport.github", Type: configregistry.TypeObject})
	return reg
}

// ghTopic builds a github-transport Topic with the given config JSON.
// Built as a struct literal (not streaming.NewTopic) on purpose: NewTopic
// runs Transport.Validate(), which rejects empty / malformed repos — but
// those are exactly the degraded configs we need to exercise the
// provisioner's repo guards against.
func ghTopic(configJSON string) streaming.Topic {
	return streaming.Topic{
		ID:             "s-gh",
		OrganizationID: provOrg,
		Name:           "s-gh",
		CreatedBy:      "w-root",
		CreatedAt:      time.Now().UTC(),
		Transport:      transport.Transport{Kind: transport.KindGitHub, Config: json.RawMessage(configJSON)},
	}
}

func failKind(t *testing.T, err error) streaming.FailKind {
	t.Helper()
	var f *streaming.Failure
	if !errors.As(err, &f) {
		t.Fatalf("error is not a *streaming.Failure: %T %v", err, err)
	}
	return f.Kind
}

func TestSplitRepo(t *testing.T) {
	t.Parallel()
	owner, name, ferr := splitRepo("octocat/hello-world")
	if ferr != nil || owner != "octocat" || name != "hello-world" {
		t.Fatalf("splitRepo(valid) = %q,%q,%v", owner, name, ferr)
	}
	if _, _, ferr := splitRepo("no-slash"); ferr == nil {
		t.Fatal("splitRepo(no slash) should fail")
	} else if ferr.Kind != streaming.FailBadRequest {
		t.Fatalf("splitRepo malformed kind = %v, want FailBadRequest", ferr.Kind)
	}
}

func TestPayloadURL(t *testing.T) {
	t.Parallel()
	p := NewWebhookProvisioner(nil, nil, "https://meta.helix.ml/")
	// Trailing slash on the base is trimmed; org + topic id are escaped.
	got := p.payloadURL("https://meta.helix.ml/", "org ab", "s-x/y")
	want := "https://meta.helix.ml/api/v1/orgs/org%20ab/topics/s-x%2Fy/github/webhook"
	if got != want {
		t.Fatalf("payloadURL = %q, want %q", got, want)
	}
}

func TestResolvePublicURL(t *testing.T) {
	t.Parallel()

	// Returns the trimmed SERVER_URL.
	p := NewWebhookProvisioner(nil, nil, "  https://server.example  ")
	if got := p.resolvePublicURL(); got != "https://server.example" {
		t.Fatalf("resolvePublicURL = %q", got)
	}

	// Empty SERVER_URL → empty.
	if got := NewWebhookProvisioner(nil, nil, "").resolvePublicURL(); got != "" {
		t.Fatalf("empty resolvePublicURL = %q", got)
	}
}

func TestEnsureWebhookSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// nil configs → error.
	if _, err := NewWebhookProvisioner(nil, nil, "").ensureWebhookSecret(ctx, provOrg); err == nil {
		t.Fatal("ensureWebhookSecret with nil configs should error")
	}

	reg := newReg(t)
	p := NewWebhookProvisioner(reg, nil, "")
	secret, err := p.ensureWebhookSecret(ctx, provOrg)
	if err != nil {
		t.Fatalf("ensureWebhookSecret: %v", err)
	}
	if len(secret) != 64 { // 32 random bytes, hex-encoded
		t.Fatalf("secret length = %d, want 64 hex chars", len(secret))
	}
	// Idempotent: a second call returns the SAME persisted secret, not a
	// fresh one (the inbound HMAC verifier and future installs must agree).
	again, err := p.ensureWebhookSecret(ctx, provOrg)
	if err != nil {
		t.Fatalf("ensureWebhookSecret (2nd): %v", err)
	}
	if again != secret {
		t.Fatalf("secret regenerated: %q != %q", again, secret)
	}
}

// TestEnsureWebhookSecret_PreservesToken pins that generating the secret
// does not clobber an existing transport.github.token (the struct round-
// trips both fields).
func TestEnsureWebhookSecret_PreservesToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newReg(t)
	if err := reg.Set(ctx, provOrg, "transport.github", `{"token":"ghp_keepme"}`); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	p := NewWebhookProvisioner(reg, nil, "")
	if _, err := p.ensureWebhookSecret(ctx, provOrg); err != nil {
		t.Fatalf("ensureWebhookSecret: %v", err)
	}
	var cfg struct {
		Token         string `json:"token"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := reg.GetObject(ctx, provOrg, "transport.github", &cfg); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if cfg.Token != "ghp_keepme" {
		t.Fatalf("token clobbered: %q", cfg.Token)
	}
	if cfg.WebhookSecret == "" {
		t.Fatal("webhook secret not persisted alongside token")
	}
}

func TestResolveToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// nil resolver → precondition failure.
	if _, ferr := NewWebhookProvisioner(nil, nil, "").resolveToken(ctx, provOrg); ferr == nil || ferr.Kind != streaming.FailPrecondition {
		t.Fatalf("nil resolver = %v", ferr)
	}
	// resolver error → internal failure.
	errResolver := func(context.Context, string) (string, error) { return "", errors.New("boom") }
	if _, ferr := NewWebhookProvisioner(nil, errResolver, "").resolveToken(ctx, provOrg); ferr == nil || ferr.Kind != streaming.FailInternal {
		t.Fatalf("error resolver = %v", ferr)
	}
	// empty token → precondition failure (no creds for the org).
	emptyResolver := func(context.Context, string) (string, error) { return "", nil }
	if _, ferr := NewWebhookProvisioner(nil, emptyResolver, "").resolveToken(ctx, provOrg); ferr == nil || ferr.Kind != streaming.FailPrecondition {
		t.Fatalf("empty resolver = %v", ferr)
	}
	// valid token passes through.
	okResolver := func(context.Context, string) (string, error) { return "tok", nil }
	tok, ferr := NewWebhookProvisioner(nil, okResolver, "").resolveToken(ctx, provOrg)
	if ferr != nil || tok != "tok" {
		t.Fatalf("ok resolver = %q,%v", tok, ferr)
	}
}

// TestInstall_PreconditionGuards covers the Install branches that fail
// before any GitHub call. The loopback refusal is the security-relevant
// one — GitHub won't deliver to localhost and we must say so clearly.
func TestInstall_PreconditionGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	okToken := func(context.Context, string) (string, error) { return "tok", nil }
	repoCfg := `{"repo":"octo/repo","events":["push"]}`

	cases := []struct {
		name       string
		publicURL  string
		token      TokenResolver
		topicJSON string
		wantKind   streaming.FailKind
	}{
		{"no public url", "", okToken, repoCfg, streaming.FailPrecondition},
		{"loopback localhost", "http://localhost:8080", okToken, repoCfg, streaming.FailPrecondition},
		{"loopback 127.0.0.1", "http://127.0.0.1:9000", okToken, repoCfg, streaming.FailPrecondition},
		{"no repo", "https://meta.helix.ml", okToken, `{}`, streaming.FailBadRequest},
		{"no token", "https://meta.helix.ml", nil, repoCfg, streaming.FailPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewWebhookProvisioner(newReg(t), tc.token, tc.publicURL)
			_, err := p.Install(ctx, provOrg, ghTopic(tc.topicJSON))
			if err == nil {
				t.Fatal("expected Install to fail before the GitHub call")
			}
			if k := failKind(t, err); k != tc.wantKind {
				t.Fatalf("kind = %v, want %v (err=%v)", k, tc.wantKind, err)
			}
		})
	}
}

// TestStatus_DegradesToUnknown covers Status's read-only contract: every
// "can't tell" case returns State="unknown" with a Detail and a nil error
// (never a hard failure) — so the detail page can render a hint instead of
// erroring.
func TestStatus_DegradesToUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	okToken := func(context.Context, string) (string, error) { return "tok", nil }
	errToken := func(context.Context, string) (string, error) { return "", errors.New("nope") }
	emptyToken := func(context.Context, string) (string, error) { return "", nil }
	repoCfg := `{"repo":"octo/repo","events":["push"]}`

	cases := []struct {
		name       string
		publicURL  string
		token      TokenResolver
		topicJSON string
	}{
		{"no repo", "https://meta.helix.ml", okToken, `{}`},
		{"malformed repo", "https://meta.helix.ml", okToken, `{"repo":"noslash"}`},
		{"no public url", "", okToken, repoCfg},
		{"nil resolver", "https://meta.helix.ml", nil, repoCfg},
		{"resolver error", "https://meta.helix.ml", errToken, repoCfg},
		{"empty token", "https://meta.helix.ml", emptyToken, repoCfg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewWebhookProvisioner(newReg(t), tc.token, tc.publicURL)
			state, err := p.Status(ctx, provOrg, ghTopic(tc.topicJSON))
			if err != nil {
				t.Fatalf("Status should degrade, not error: %v", err)
			}
			if state.State != "unknown" {
				t.Fatalf("state = %q, want unknown", state.State)
			}
			if strings.TrimSpace(state.Detail) == "" {
				t.Fatal("unknown state must carry a Detail")
			}
		})
	}
}
