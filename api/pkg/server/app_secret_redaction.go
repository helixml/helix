package server

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// An agent config holds credentials in many places: app secrets, API tool
// headers and query parameters, MCP server headers and environment, Zapier and
// Azure DevOps keys, knowledge crawler credentials and chat integration
// tokens. Agents are readable by more people than can edit them (a global
// agent by every user, an org agent by every member granted read access), so
// readers get these values masked and only callers who may update the agent
// see them.
//
// Every credential is addressed by a stable path so that a masked value sent
// back on update can be matched to the stored value it stands for.

// agentSecretTransform maps one credential, identified by its path, to the
// value to keep. Returning "" for a map entry removes the entry.
type agentSecretTransform func(path, value string) string

// transformAgentSecrets applies fn to every credential in cfg, in place.
func transformAgentSecrets(cfg *types.AgentConfig, fn agentSecretTransform) {
	cfg.Secrets = transformSecretMap("secrets", cfg.Secrets, fn)

	for i := range cfg.Helix.Assistants {
		assistant := &cfg.Helix.Assistants[i]
		base := "assistants/" + assistantSecretKey(assistant)

		for j := range assistant.APIs {
			api := &assistant.APIs[j]
			p := base + "/apis/" + api.Name
			api.Headers = transformSecretMap(p+"/headers", api.Headers, fn)
			api.Query = transformSecretMap(p+"/query", api.Query, fn)
		}
		for j := range assistant.MCPs {
			mcp := &assistant.MCPs[j]
			p := base + "/mcps/" + mcp.Name
			mcp.Headers = transformSecretMap(p+"/headers", mcp.Headers, fn)
			mcp.Env = transformSecretMap(p+"/env", mcp.Env, fn)
		}
		for j := range assistant.Zapier {
			z := &assistant.Zapier[j]
			z.APIKey = transformSecretValue(base+"/zapier/"+z.Name+"/api_key", z.APIKey, fn)
		}
		assistant.AzureDevOps.PersonalAccessToken = transformSecretValue(base+"/azure_devops/pat", assistant.AzureDevOps.PersonalAccessToken, fn)

		for _, k := range assistant.Knowledge {
			if k != nil {
				transformKnowledgeSourceSecrets(base+"/knowledge/"+k.Name, &k.Source, fn)
			}
		}

		for _, tool := range assistant.Tools {
			if tool == nil {
				continue
			}
			p := base + "/tools/" + tool.Name
			if c := tool.Config.API; c != nil {
				c.Headers = transformSecretMap(p+"/headers", c.Headers, fn)
				c.Query = transformSecretMap(p+"/query", c.Query, fn)
			}
			if c := tool.Config.MCP; c != nil {
				c.Headers = transformSecretMap(p+"/headers", c.Headers, fn)
			}
			if c := tool.Config.Zapier; c != nil {
				c.APIKey = transformSecretValue(p+"/api_key", c.APIKey, fn)
			}
			if c := tool.Config.AzureDevOps; c != nil {
				c.PersonalAccessToken = transformSecretValue(p+"/pat", c.PersonalAccessToken, fn)
			}
		}
	}

	for i := range cfg.Helix.Triggers {
		t := &cfg.Helix.Triggers[i]
		// Triggers have no ID or name, so they are keyed by kind; two
		// triggers of one kind share a path, which restore treats as
		// ambiguous.
		p := "triggers"
		if t.Slack != nil {
			t.Slack.AppToken = transformSecretValue(p+"/slack/app_token", t.Slack.AppToken, fn)
			t.Slack.BotToken = transformSecretValue(p+"/slack/bot_token", t.Slack.BotToken, fn)
		}
		if t.Teams != nil {
			t.Teams.AppPassword = transformSecretValue(p+"/teams/app_password", t.Teams.AppPassword, fn)
		}
		if t.Crisp != nil {
			t.Crisp.Token = transformSecretValue(p+"/crisp/token", t.Crisp.Token, fn)
		}
	}
}

func transformKnowledgeSourceSecrets(path string, source *types.KnowledgeSource, fn agentSecretTransform) {
	if source.Web == nil {
		return
	}
	// Copy the pointed-to structs so a stored config sharing them is not
	// modified.
	web := *source.Web
	web.Auth.Password = transformSecretValue(path+"/web_password", web.Auth.Password, fn)
	if web.Crawler != nil && web.Crawler.Firecrawl != nil {
		crawler := *web.Crawler
		firecrawl := *crawler.Firecrawl
		firecrawl.APIKey = transformSecretValue(path+"/firecrawl_api_key", firecrawl.APIKey, fn)
		crawler.Firecrawl = &firecrawl
		web.Crawler = &crawler
	}
	source.Web = &web
}

// maskKnowledgeSourceCredentials masks the crawler credentials of a knowledge
// source in place.
func maskKnowledgeSourceCredentials(source *types.KnowledgeSource) {
	transformKnowledgeSourceSecrets("", source, func(_, _ string) string { return maskedSecret })
}

// assistantSecretKey identifies an assistant across a read and the update
// that follows it. Positions are not used: they shift when an entry is
// removed. Unnamed assistants share a key, which restore treats as ambiguous.
func assistantSecretKey(a *types.AssistantConfig) string {
	if a.ID != "" {
		return "id:" + a.ID
	}
	return "name:" + a.Name
}

func transformSecretValue(path, value string, fn agentSecretTransform) string {
	if value == "" {
		return ""
	}
	return fn(path, value)
}

// transformSecretMap returns a new map so a caller never mutates a map it
// shares with the stored config.
func transformSecretMap(path string, values map[string]string, fn agentSecretTransform) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		if value == "" {
			out[name] = ""
			continue
		}
		if v := fn(path+"/"+name, value); v != "" {
			out[name] = v
		}
	}
	return out
}

// cloneAgentConfig deep-copies a config through its stored JSON form.
func cloneAgentConfig(cfg types.AgentConfig) (types.AgentConfig, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return types.AgentConfig{}, err
	}
	var out types.AgentConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		return types.AgentConfig{}, err
	}
	return out, nil
}

// maskedAgentConfig returns a copy of cfg with every credential replaced by
// the masked placeholder. If the copy fails the credentials are dropped
// rather than returned.
func maskedAgentConfig(cfg types.AgentConfig) types.AgentConfig {
	out, err := cloneAgentConfig(cfg)
	if err != nil {
		log.Error().Err(err).Msg("failed to copy agent config for redaction")
		return types.AgentConfig{Helix: types.AgentHelixConfig{Name: cfg.Helix.Name, Description: cfg.Helix.Description}}
	}
	transformAgentSecrets(&out, func(_, _ string) string { return maskedSecret })
	return out
}

// stripAgentSecrets removes every credential from cfg in place.
func stripAgentSecrets(cfg *types.AgentConfig) {
	transformAgentSecrets(cfg, func(_, _ string) string { return "" })
}

// restoreMaskedAgentSecrets resolves masked placeholders in an update: each
// one is replaced by the stored value at the same path. A placeholder is
// dropped when there is no stored value, or when its path is ambiguous
// (several entries share a name or kind in either config), so a value is
// never assigned to an entry it did not come from. Any other value is kept as
// sent.
func restoreMaskedAgentSecrets(stored types.AgentConfig, update *types.AgentConfig) {
	previous := map[string]string{}
	storedCount := map[string]int{}
	if storedCopy, err := cloneAgentConfig(stored); err == nil {
		transformAgentSecrets(&storedCopy, func(path, value string) string {
			previous[path] = value
			storedCount[path]++
			return value
		})
	}
	updateCount := map[string]int{}
	if updateCopy, err := cloneAgentConfig(*update); err == nil {
		transformAgentSecrets(&updateCopy, func(path, value string) string {
			updateCount[path]++
			return value
		})
	}
	transformAgentSecrets(update, func(path, value string) string {
		if !isMaskedSecret(value) {
			return value
		}
		if storedCount[path] != 1 || updateCount[path] != 1 {
			return ""
		}
		return previous[path]
	})
}

// canUpdateApp reports whether the caller may edit the agent and therefore
// see its credentials.
func (s *HelixAPIServer) canUpdateApp(ctx context.Context, user *types.User, app *types.App) bool {
	if user == nil {
		return false
	}
	return s.authorizeUserToApp(ctx, user, app, types.ActionUpdate) == nil
}

// redactAppForUser masks the agent's credentials unless the caller may edit
// it. The app is modified in place; it must be a copy the caller owns, such
// as one just loaded from the store.
func (s *HelixAPIServer) redactAppForUser(ctx context.Context, user *types.User, app *types.App) {
	if app == nil || s.canUpdateApp(ctx, user, app) {
		return
	}
	app.Config = maskedAgentConfig(app.Config)
}
