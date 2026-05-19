package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	helixorgui "github.com/helixml/helix-org/server/ui"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// newHelixOrgAgentPickerHandler returns an HTTP handler that lets the
// operator pick which Helix agent drives the embedded helix-org chat
// surface. The picker reads the user's available agents directly from
// Helix's REST API using the api_key configured under helix.api_key,
// and persists the choice as chat.app_id in helix-org's config store.
//
// Because the bridge reads chat.app_id dynamically per chat send (see
// helix_org_chat.go), no restart is needed after picking — the next
// /ui/chat/send picks up the new agent.
//
// Mounted at /ui/alpha-agents under the same alpha-gated subtree as
// the rest of the helix-org UI.
func newHelixOrgAgentPickerHandler(reg *config.Registry, st helixstore.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			renderAgentPicker(ctx, w, r, reg, st)
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form: "+err.Error(), http.StatusBadRequest)
				return
			}
			appID := strings.TrimSpace(r.FormValue("app_id"))
			if appID == "" {
				http.Error(w, "app_id is required", http.StatusBadRequest)
				return
			}
			payload, err := json.Marshal(appID)
			if err != nil {
				http.Error(w, "encode value: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if err := reg.Set(ctx, "chat.app_id", string(payload), domain.WorkerID("w-owner")); err != nil {
				http.Error(w, "save chat.app_id: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Info().Str("app_id", appID).Msg("helix-org chat.app_id updated via picker")

			// Attach helix-org's MCP server to the picked agent so the
			// agent can actually drive the org graph. We go through the
			// Helix MCP gateway (/api/v1/mcp/helix-org/...) rather than
			// directly to /api/v1/org/... so the agent's MCP client
			// doesn't have to thread the embedded helix-org auth gate —
			// the gateway sits behind standard Helix api_key auth and
			// the backend re-checks the alpha-feature flag.
			//
			// We bake the picking user's api_key into the MCP entry's
			// headers so the agent's MCP client (which doesn't carry an
			// ambient Helix session) presents valid credentials on each
			// tool call. The agent is owned by the same user, so this is
			// the user's own key on their own agent — same trust scope.
			baseURL, _ := reg.GetString(ctx, "helix.url")
			orgURL, _ := reg.GetString(ctx, "helix.org_url")
			attachURL := strings.TrimRight(orgURL, "/")
			if attachURL == "" {
				attachURL = strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org"
			}
			// Path after the gateway segment is irrelevant — the backend
			// rewrites everything onto the owner Worker. Pick a stable
			// suffix so the URL reads sensibly in stored config.
			mcpURL := attachURL + "/workers/w-owner/mcp"

			user := getRequestUser(r)
			var userAPIKey string
			if hasUser(user) {
				if k, err := resolveUserHelixAPIKey(ctx, st, user.ID); err == nil {
					userAPIKey = k
				} else {
					log.Warn().Err(err).Str("user_id", user.ID).Msg("agent picked but per-user api key resolve failed — MCP attach will fall back to service key")
				}
			}
			serviceKey, _ := reg.GetString(ctx, "helix.api_key")
			if userAPIKey == "" {
				userAPIKey = serviceKey
			}
			client, cerr := helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: serviceKey})
			if cerr != nil {
				log.Warn().Err(cerr).Msg("agent picked but helix client init failed — MCP not attached")
				http.Redirect(w, r, "/ui/alpha-agents?saved=1", http.StatusSeeOther)
				return
			}
			headers := map[string]string{}
			if userAPIKey != "" {
				headers["Authorization"] = "Bearer " + userAPIKey
			}
			if err := helixclient.AttachMCPToAppWithHeaders(ctx, client, appID, "helix-org", "http", mcpURL, headers); err != nil {
				log.Warn().Err(err).Str("app_id", appID).Str("mcp_url", mcpURL).Msg("attach helix-org MCP to agent failed — agent will chat but can't call org tools")
			} else {
				log.Info().Str("app_id", appID).Str("mcp_url", mcpURL).Msg("attached helix-org MCP to picked agent")
			}
			http.Redirect(w, r, "/ui/alpha-agents?saved=1", http.StatusSeeOther)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func renderAgentPicker(ctx context.Context, w http.ResponseWriter, r *http.Request, reg *config.Registry, st helixstore.Store) {
	current, _ := reg.GetString(ctx, "chat.app_id")
	baseURL, _ := reg.GetString(ctx, "helix.url")

	// The bearer is already injected per-request by
	// withHelixUserBearer; we look it back up here only to scope the
	// /api/v1/apps query to the logged-in user's first org. Falling
	// back to helix.api_key keeps the picker functional for callers
	// that arrived without a session (e.g. integration tests).
	user := getRequestUser(r)
	bearer := ""
	var orgID string
	if hasUser(user) {
		if k, err := resolveUserHelixAPIKey(ctx, st, user.ID); err == nil {
			bearer = k
		}
		if memberships, err := st.ListOrganizationMemberships(ctx, &helixstore.ListOrganizationMembershipsQuery{UserID: user.ID}); err == nil && len(memberships) > 0 {
			orgID = memberships[0].OrganizationID
		}
	}
	if bearer == "" {
		bearer, _ = reg.GetString(ctx, "helix.api_key")
	}

	var renderErr string
	var agents []agentSummary
	switch {
	case bearer == "":
		renderErr = "no api key available — log in and reload."
	case baseURL == "":
		renderErr = "helix.url is not set — configure it under /ui/settings first."
	default:
		fetched, err := listHelixAgents(ctx, baseURL, bearer, orgID)
		if err != nil {
			renderErr = err.Error()
		} else {
			agents = fetched
		}
	}

	rows := make([]helixorgui.AlphaAgentRow, 0, len(agents))
	for _, a := range agents {
		providerModel := ""
		if a.Provider != "" || a.Model != "" {
			providerModel = a.Provider
			if a.Model != "" {
				if providerModel != "" {
					providerModel += "/"
				}
				providerModel += a.Model
			}
		}
		rows = append(rows, helixorgui.AlphaAgentRow{
			ID:               a.ID,
			Name:             a.Name,
			AgentType:        a.AgentType,
			Runtime:          a.Runtime,
			HasRuntime:       a.Runtime != "",
			ProviderModel:    providerModel,
			HasProviderModel: providerModel != "",
			IsCurrent:        a.ID == current,
		})
	}
	page := &helixorgui.AlphaAgentsPage{
		CurrentID: current,
		Agents:    rows,
	}
	if r.URL.Query().Get("saved") != "" {
		page.Flash = "Saved."
	}
	if renderErr != "" {
		page.Error = renderErr
	}
	helixorgui.RenderAlphaAgents(w, "w-owner", nil, page)
}

// agentSummary is the shape rendered on the picker page.
type agentSummary struct {
	ID        string
	Name      string
	Provider  string
	Model     string
	AgentType string
	Runtime   string
}

// listHelixAgents calls the surrounding Helix's /api/v1/apps using the
// configured api_key. We do this server-side so the picker doesn't
// depend on the operator's browser session carrying the same identity
// as helix.api_key.
func listHelixAgents(ctx context.Context, baseURL, apiKey, orgID string) ([]agentSummary, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/apps"
	if orgID != "" {
		endpoint += "?organization_id=" + orgID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", endpoint, resp.StatusCode)
	}
	var raw []rawHelixApp
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode apps: %w", err)
	}
	out := make([]agentSummary, 0, len(raw))
	for _, a := range raw {
		name := a.Config.Helix.Name
		var provider, model, agentType, runtime string
		if len(a.Config.Helix.Assistants) > 0 {
			as := a.Config.Helix.Assistants[0]
			provider = as.Provider
			model = as.Model
			agentType = as.AgentType
			runtime = as.CodeAgentRuntime
		}
		if name == "" {
			name = "(unnamed)"
		}
		out = append(out, agentSummary{
			ID:        a.ID,
			Name:      name,
			Provider:  provider,
			Model:     model,
			AgentType: agentType,
			Runtime:   runtime,
		})
	}
	return out, nil
}

// rawHelixApp mirrors just the fields of /api/v1/apps the picker
// needs. Avoiding a hard dependency on api/pkg/types.App keeps the
// picker decoupled from unrelated type churn.
type rawHelixApp struct {
	ID     string `json:"id"`
	Config struct {
		Helix struct {
			Name       string `json:"name"`
			Assistants []struct {
				Provider         string `json:"provider"`
				Model            string `json:"model"`
				AgentType        string `json:"agent_type"`
				CodeAgentRuntime string `json:"code_agent_runtime"`
			} `json:"assistants"`
		} `json:"helix"`
	} `json:"config"`
}

