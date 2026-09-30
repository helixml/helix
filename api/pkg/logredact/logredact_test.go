package logredact

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const (
	ghInstallation = "ghs_16C7e42F292c6912E7710c838347Ae178B4a"
	ghPAT          = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	ghFineGrained  = "github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz"
)

func TestString(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"installation token", "token " + ghInstallation + " minted", "token [REDACTED] minted"},
		{"classic pat", "GH_TOKEN " + ghPAT, "GH_TOKEN [REDACTED]"},
		{"fine-grained pat", "pat=" + ghFineGrained, "pat=[REDACTED]"},
		{"helix api key", "key hl-AbCdEfGhIjKlMnOpQrStUvWxYz012345==", "key [REDACTED]"},
		{
			"url userinfo",
			"fetch failed: exit status 128 - fatal: unable to access 'https://x-access-token:secretvalue@github.com/helixml/helix.git/'",
			"fetch failed: exit status 128 - fatal: unable to access 'https://[REDACTED]@github.com/helixml/helix.git/'",
		},
		{"url token as username", "https://sometoken@github.com/o/r", "https://[REDACTED]@github.com/o/r"},
		{"authorization header", "Authorization: token abc123def", "Authorization: [REDACTED]"},
		{"bearer", `curl -H "Authorization: Bearer abc.def.ghi"`, `curl -H "Authorization: [REDACTED]"`},
		{"bare bearer", "sent Bearer abcdefghijkl", "sent Bearer [REDACTED]"},
		{"json authorization", `{"Authorization":"Bearer xyz"}`, `{"Authorization":"[REDACTED]"}`},
		{"query token", "GET /repos?access_token=abc123&page=2", "GET /repos?access_token=[REDACTED]&page=2"},
		{"env assignment", "env GH_TOKEN=abc123 git push", "env GH_TOKEN=[REDACTED] git push"},
		{"plain url untouched", "cloning https://github.com/helixml/helix.git", "cloning https://github.com/helixml/helix.git"},
		{"plain text untouched", "max_tokens=100 prompt_tokens=5", "max_tokens=100 prompt_tokens=5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := String(tc.in); got != tc.want {
				t.Fatalf("String(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

type config struct {
	Repo  string
	Token string
}

func TestSlogHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewTextHandler(&buf, nil))).
		With("remote", "https://x-access-token:"+ghInstallation+"@github.com/o/r")

	logger.Info("minted "+ghPAT,
		"err", fmt.Errorf("git push: %w", errors.New("https://oauth2:"+ghPAT+"@github.com")),
		"cfg", config{Repo: "o/r", Token: ghInstallation},
		slog.Group("req", "auth", "Bearer "+ghFineGrained),
		"count", 3,
	)

	out := buf.String()
	for _, secret := range []string{ghInstallation, ghPAT, ghFineGrained} {
		if strings.Contains(out, secret) {
			t.Fatalf("log output leaked %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"count=3", "req.auth=", "cfg=", "remote=https://[REDACTED]@github.com/o/r"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestWriter(t *testing.T) {
	var buf bytes.Buffer
	line := "ERR clone failed error=\"https://x-access-token:" + ghInstallation + "@github.com/o/r\"\n"
	n, err := NewWriter(&buf).Write([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(line) {
		t.Fatalf("Write returned %d, want %d", n, len(line))
	}
	if strings.Contains(buf.String(), ghInstallation) {
		t.Fatalf("writer leaked token: %s", buf.String())
	}
}
