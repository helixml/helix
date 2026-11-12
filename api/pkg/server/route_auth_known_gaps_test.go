package server

// knownEnforcementGaps lists "<METHOD path> | <probe>" pairs that are not
// refused today. Every entry is a defect to fix, not an exception: the
// enforcement test fails when one starts refusing, so the list only shrinks.
//
// All current entries are one defect: requireUser admits the deployment
// runner token as the pseudo-user "runner-system", so every user route that
// does not itself refuse runner tokens serves it (lists, creates and updates
// rows owned by "runner-system", or fails inside the handler). Routes whose
// handlers serve sandbox hosts on purpose are marked alsoRunner() instead.
const gapRunnerTokenAsUser = "runner token admitted by requireUser as user runner-system"

var knownEnforcementGaps = map[string]string{
	"DELETE /api/v1/api_keys | runner token":                                            gapRunnerTokenAsUser, // answered 500
	"DELETE /api/v1/oauth/connections/{id} | runner token":                              gapRunnerTokenAsUser, // answered 500
	"DELETE /api/v1/oauth/providers/{id} | runner token":                                gapRunnerTokenAsUser, // answered 500
	"DELETE /api/v1/projects/{id}/pin | runner token":                                   gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/agent-tools | runner token":                                            gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/agents | runner token":                                                 gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/agents/{id}/user-access | runner token":                                gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/api_keys | runner token":                                               gapRunnerTokenAsUser, // did not answer within 5s
	"GET /api/v1/apps | runner token":                                                   gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/apps/{id}/user-access | runner token":                                  gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/attention-events | runner token":                                       gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/bandwidth-probe | runner token":                                        gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/claude-subscriptions | runner token":                                   gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/claude-subscriptions/models | runner token":                            gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/codex-subscriptions | runner token":                                    gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/context-menu | runner token":                                           gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/external-agents/sync | runner token":                                   gapRunnerTokenAsUser, // panicked
	"GET /api/v1/git-provider-connections | runner token":                               gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/git/repositories | runner token":                                       gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/helix-models | runner token":                                           gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/knowledge | runner token":                                              gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/oauth/connections | runner token":                                      gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/oauth/connections/{id} | runner token":                                 gapRunnerTokenAsUser, // answered 500
	"GET /api/v1/oauth/connections/{id}/repositories | runner token":                    gapRunnerTokenAsUser, // answered 500
	"GET /api/v1/oauth/connections/{id}/test | runner token":                            gapRunnerTokenAsUser, // answered 500
	"GET /api/v1/oauth/flow/start/{provider_id} | runner token":                         gapRunnerTokenAsUser, // panicked
	"GET /api/v1/oauth/providers | runner token":                                        gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/oauth/providers/{id} | runner token":                                   gapRunnerTokenAsUser, // answered 500
	"GET /api/v1/organizations | runner token":                                          gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/projects | runner token":                                               gapRunnerTokenAsUser, // panicked
	"GET /api/v1/providers | runner token":                                              gapRunnerTokenAsUser, // panicked
	"GET /api/v1/quotas | runner token":                                                 gapRunnerTokenAsUser, // panicked
	"GET /api/v1/repositories/without-projects | runner token":                          gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/sample-projects/simple | runner token":                                 gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/sandbox-desktop-types | runner token":                                  gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/sandbox-runtimes | runner token":                                       gapRunnerTokenAsUser, // panicked
	"GET /api/v1/sandboxes/{id}/containers/{session_id}/blkio | runner token":           gapRunnerTokenAsUser, // panicked
	"GET /api/v1/search | runner token":                                                 gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/secrets | runner token":                                                gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/sessions | runner token":                                               gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/skills | runner token":                                                 gapRunnerTokenAsUser, // panicked
	"GET /api/v1/skills/{id} | runner token":                                            gapRunnerTokenAsUser, // panicked
	"GET /api/v1/specs/sample-types | runner token":                                     gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/status | runner token":                                                 gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/triggers | runner token":                                               gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/triggers/{trigger_id}/executions | runner token":                       gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/usage | runner token":                                                  gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/me/chat-settings | runner token":                                 gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/me/guidelines | runner token":                                    gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/me/guidelines-history | runner token":                            gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/me/pinned-chats | runner token":                                  gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/me/pinned-projects | runner token":                               gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/users/token-usage | runner token":                                      gapRunnerTokenAsUser, // answered 200
	"GET /api/v1/v1/models | runner token":                                              gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/agents | runner token":                                                gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/api_keys | runner token":                                              gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/apps | runner token":                                                  gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/attention-events/dismiss-all | runner token":                          gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/auth/update | runner token":                                           gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/claude-subscriptions/oauth/start | runner token":                      gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/codex-subscriptions/start-login | runner token":                       gapRunnerTokenAsUser, // panicked
	"POST /api/v1/external-agents/{sessionID}/configure-pending-session | runner token": gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/oauth/connections/{id}/refresh | runner token":                        gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/oauth/providers | runner token":                                       gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/oauth/sharepoint/resolve-site | runner token":                         gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/secrets | runner token":                                               gapRunnerTokenAsUser, // answered 200
	"POST /api/v1/skills/reload | runner token":                                         gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/skills/validate | runner token":                                       gapRunnerTokenAsUser, // panicked
	"POST /api/v1/subscription/manage | runner token":                                   gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/subscription/new | runner token":                                      gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/top-ups/new | runner token":                                           gapRunnerTokenAsUser, // answered 500
	"POST /api/v1/users/me/onboarding | runner token":                                   gapRunnerTokenAsUser, // answered 500
	"PUT /api/v1/oauth/providers/{id} | runner token":                                   gapRunnerTokenAsUser, // answered 500
	"PUT /api/v1/users/me/chat-settings | runner token":                                 gapRunnerTokenAsUser, // answered 200
	"PUT /api/v1/users/me/color-scheme | runner token":                                  gapRunnerTokenAsUser, // answered 200
	"PUT /api/v1/users/me/guidelines | runner token":                                    gapRunnerTokenAsUser, // answered 200
}
