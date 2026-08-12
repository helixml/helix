package types

import (
	"strings"
	"testing"
	"time"
)

func TestNewPushError(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		raw         string
		account     string
		repo        string
		wantCauseIn string // substring expected in Cause
		wantNextIn  string // substring expected in NextStep
	}{
		{
			name:        "github private repo 404",
			raw:         "remote: Repository not found.\nfatal: repository 'https://github.com/helixml/find-ai.git/' not found",
			account:     "@linuxrecruit",
			repo:        "helixml/find-ai",
			wantCauseIn: "@linuxrecruit can't access helixml/find-ai",
			wantNextIn:  "Switch to a VCS account",
		},
		{
			name:        "forbidden",
			raw:         "remote: Permission to helixml/find-ai.git denied (403)",
			account:     "@bob",
			repo:        "helixml/find-ai",
			wantCauseIn: "doesn't have write permission",
			wantNextIn:  "write access",
		},
		{
			name:        "auth failure",
			raw:         "fatal: Authentication failed for 'https://github.com/x/y.git'",
			account:     "@bob",
			repo:        "x/y",
			wantCauseIn: "Authentication failed",
			wantNextIn:  "Reconnect",
		},
		{
			name:        "unknown error, empty account",
			raw:         "fatal: some other error",
			account:     "",
			repo:        "x/y",
			wantCauseIn: "your connected account",
			wantNextIn:  "switch accounts",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pe := NewPushError(ExternalRepositoryTypeGitHub, tc.account, tc.repo, tc.raw, now)
			if pe.Provider != ExternalRepositoryTypeGitHub {
				t.Errorf("provider = %q, want github", pe.Provider)
			}
			if pe.RawMessage != tc.raw {
				t.Errorf("raw message not preserved verbatim")
			}
			if !pe.FailedAt.Equal(now) {
				t.Errorf("FailedAt = %v, want %v", pe.FailedAt, now)
			}
			if !strings.Contains(pe.Cause, tc.wantCauseIn) {
				t.Errorf("Cause = %q, want substring %q", pe.Cause, tc.wantCauseIn)
			}
			if !strings.Contains(pe.NextStep, tc.wantNextIn) {
				t.Errorf("NextStep = %q, want substring %q", pe.NextStep, tc.wantNextIn)
			}
		})
	}
}
