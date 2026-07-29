package webservice

import "strings"

// deploySignature maps a fatal marker in a project's startup/deploy log to an
// operator-readable explanation.
type deploySignature struct {
	marker  string
	message string
}

// deploySignatures are matched against the tail of /data/.helix-webservice.log
// when a deploy fails its readiness check. Order matters: the most specific
// diagnosis wins, so the corrupt-checkpoint signatures are checked before the
// generic "PANIC:".
//
// The motivating case: we-find.ai's nested Postgres died with a corrupt WAL
// checkpoint and the deploy reported only "the app never started listening on
// port 8080". That reads like a code problem, so the platform kept redeploying
// the same commit — which cannot possibly fix a corrupt data volume — and the
// operator had no way to tell without SSHing to a runner.
var deploySignatures = []deploySignature{
	{
		marker:  "could not locate a valid checkpoint record",
		message: "the app's nested database will not start: its write-ahead log has a corrupt checkpoint record. This is a DATA problem, not a code problem — redeploying the same commit cannot fix it. The data volume needs recovery from a backup by an operator",
	},
	{
		marker:  "invalid resource manager ID",
		message: "the app's nested database will not start: its write-ahead log is corrupt. This is a DATA problem, not a code problem — redeploying cannot fix it. The data volume needs recovery from a backup by an operator",
	},
	{
		marker:  "database system was interrupted",
		message: "the app's nested database was shut down uncleanly and could not recover on restart. Check the deploy log; the data volume may need recovery from a backup",
	},
	{
		marker:  "dependency failed to start",
		message: "a container the app depends on never became healthy, so the app was never started. Check that dependency's logs in the deploy log",
	},
	{
		marker:  "is unhealthy",
		message: "a container in the app's stack never passed its healthcheck. Check the deploy log for that container's output",
	},
	{
		marker:  "PANIC:",
		message: "a container in the app's stack panicked on startup. Check the deploy log for the panic message",
	},
	{
		marker:  "no space left on device",
		message: "the runner ran out of disk while starting the app. An operator needs to free space on the host",
	},
}

// classifyDeployFailure returns a specific, human-readable reason for a failed
// deploy when the log tail matches a known fatal signature, or "" when it does
// not. An empty result means the caller keeps its existing generic error — a
// signature must never turn an unrecognised failure into a false diagnosis.
func classifyDeployFailure(logTail string) string {
	if logTail == "" {
		return ""
	}
	for _, sig := range deploySignatures {
		if strings.Contains(logTail, sig.marker) {
			return sig.message
		}
	}
	return ""
}
