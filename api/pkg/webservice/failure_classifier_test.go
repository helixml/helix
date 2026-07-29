package webservice

import (
	"strings"
	"testing"
)

func TestClassifyDeployFailure(t *testing.T) {
	cases := []struct {
		name    string
		logTail string
		// wantContains is "" when the failure must stay unclassified.
		wantContains string
	}{
		{
			// The actual we-find.ai log. This is the case the whole classifier
			// exists for: the platform kept redeploying the same commit against
			// a corrupt data volume, which could never work.
			name: "we-find.ai corrupt checkpoint",
			logTail: `find-ai-db-1  | LOG:  database system was interrupted; last known up at 2026-07-23 07:42:09 UTC
find-ai-db-1  | PANIC:  could not locate a valid checkpoint record
find-ai-db-1  | startup process (PID 30) was terminated by signal 6: Aborted`,
			wantContains: "corrupt checkpoint record",
		},
		{
			name:         "corrupt WAL resource manager id",
			logTail:      "PANIC:  invalid resource manager ID in primary checkpoint record",
			wantContains: "write-ahead log is corrupt",
		},
		{
			name:         "unhealthy dependency",
			logTail:      "Error response from daemon: dependency failed to start: container find-ai-db-1 is unhealthy",
			wantContains: "never became healthy",
		},
		{
			name:         "generic panic",
			logTail:      "app-1  | PANIC: runtime error: invalid memory address",
			wantContains: "panicked on startup",
		},
		{
			name:         "disk full",
			logTail:      "write /var/lib/postgresql/data/base/1: no space left on device",
			wantContains: "ran out of disk",
		},
		{
			// A plain application bug must NOT be given a database diagnosis.
			// A false explanation is worse than none: it sends the operator to
			// the wrong system.
			name:         "unrecognised failure stays unclassified",
			logTail:      "app-1  | Error: listen EADDRINUSE: address already in use :::3000",
			wantContains: "",
		},
		{
			name:         "empty log stays unclassified",
			logTail:      "",
			wantContains: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyDeployFailure(c.logTail)
			if c.wantContains == "" {
				if got != "" {
					t.Errorf("expected no classification, got %q", got)
				}
				return
			}
			if !strings.Contains(got, c.wantContains) {
				t.Errorf("classification %q does not contain %q", got, c.wantContains)
			}
		})
	}
}

// TestCorruptCheckpointBeatsGenericPanic: the we-find.ai log contains BOTH
// "PANIC:" and the corrupt-checkpoint marker. The specific diagnosis must win,
// otherwise the operator is told "something panicked" instead of "your data
// volume needs recovery".
func TestCorruptCheckpointBeatsGenericPanic(t *testing.T) {
	logTail := "PANIC:  could not locate a valid checkpoint record"
	got := classifyDeployFailure(logTail)
	if !strings.Contains(got, "corrupt checkpoint record") {
		t.Fatalf("specific signature lost to the generic one: %q", got)
	}
	if !strings.Contains(got, "DATA problem") {
		t.Errorf("diagnosis should tell the operator this is not a code problem: %q", got)
	}
}
