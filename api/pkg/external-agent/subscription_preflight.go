package external_agent

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// verifySubscriptionCredentials rejects a desktop start when the session's code
// agent authenticates with the user's own Claude/ChatGPT subscription but no
// such subscription is reachable from that session.
//
// Without this check the failure is silent and terminal: the container boots
// fine, settings-sync-daemon declines to register the ACP agent server because
// it has no credentials file to point at, and Zed's thread creation retries ten
// times and gives up. The interaction stays in `waiting` forever and the UI
// shows a spinner with no error. Failing here surfaces the reason to the caller,
// which marks the spec task failed or returns the message to the browser.
func (h *HydraExecutor) verifySubscriptionCredentials(ctx context.Context, agent *types.DesktopAgent) error {
	if h.store == nil || agent.SessionID == "" {
		return nil
	}
	session, err := h.store.GetSession(ctx, agent.SessionID)
	if err != nil || session == nil {
		// Session lookup failures are handled downstream; don't mask them here.
		return nil
	}
	// Login and exploratory desktops have no parent app — and the Codex/Claude
	// login flows start exactly such a desktop to *obtain* the subscription, so
	// they must never be gated on already having one.
	if session.ParentApp == "" && session.Metadata.SpecTaskID == "" {
		return nil
	}
	var runtime types.CodeAgentRuntime
	var credentialType types.CodeAgentCredentialType
	if session.Metadata.SpecTaskID != "" {
		task, err := h.store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
		if err != nil || task == nil || task.CodeAgentConfig == nil {
			return nil
		}
		runtime = task.CodeAgentConfig.Runtime
		credentialType = task.CodeAgentConfig.CredentialType
	} else {
		app, err := h.store.GetApp(ctx, session.ParentApp)
		if err != nil || app == nil {
			return nil
		}
		for i := range app.Config.Helix.Assistants {
			assistant := &app.Config.Helix.Assistants[i]
			if assistant.AgentType == types.AgentTypeZedExternal {
				runtime = assistant.CodeAgentRuntime
				credentialType = assistant.CodeAgentCredentialType
				break
			}
		}
	}
	if !credentialType.IsSubscription() {
		return nil
	}

	switch runtime {
	case types.CodeAgentRuntimeClaudeCode:
		sub, err := h.store.GetSessionClaudeSubscription(ctx, session)
		if err != nil || sub == nil || sub.Status != "active" {
			return missingSubscriptionError(types.SubscriptionProviderClaude, "Claude", session)
		}
	case types.CodeAgentRuntimeCodexCLI:
		sub, err := h.store.GetEffectiveCodexSubscription(ctx, session.Owner, session.OrganizationID)
		if err != nil || sub == nil || sub.Status != "active" {
			return missingSubscriptionError(types.SubscriptionProviderCodex, "ChatGPT", session)
		}
	}
	return nil
}

func missingSubscriptionError(provider, label string, session *types.Session) error {
	return &types.MissingSubscriptionError{
		Provider: provider,
		Label:    label,
		Owner:    session.Owner,
		OrgID:    session.OrganizationID,
	}
}
