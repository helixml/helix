package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
)

// A browser ticket lets an app that embeds Helix (HelixOS) connect a person's
// GitHub or GitLab in a popup, without that person signing in to Helix.
//
// The OAuth callback normally requires the browser finishing the flow to be
// signed in to Helix as the user who started it: otherwise a link carrying
// someone else's state would attach the visitor's provider account to that
// someone. A ticket gives the same guarantee without a Helix session:
//
//  1. The app, holding the person's own API key, asks for a ticket. That
//     starts the flow for the key's user and returns a link signed by this
//     server, valid for oauthBrowserTicketTTL.
//  2. The app opens the link in a popup. Helix checks the signature and that
//     the flow is still pending, binds the popup's browser to the flow with an
//     HttpOnly cookie holding the flow's state, and redirects to the provider.
//  3. The provider redirects back to the callback, which completes the flow
//     when that browser's cookie names the same state.
//
// So the flow can only be finished by the browser that opened the ticket, and
// the ticket only comes from someone holding the user's key.
const (
	oauthBrowserTicketTTL  = 2 * time.Minute
	oauthBrowserFlowCookie = "helix_oauth_flow"
)

type oauthBrowserTicket struct {
	AuthURL string `json:"a"`
	State   string `json:"s"`
	Expires int64  `json:"e"`
}

// oauthBrowserKey signs tickets. It is generated per process: a ticket lives
// two minutes, so a restart only means clicking Connect again.
func (s *HelixAPIServer) oauthBrowserKey() []byte {
	s.oauthBrowserKeyOnce.Do(func() {
		s.oauthBrowserKeyBytes = make([]byte, 32)
		if _, err := rand.Read(s.oauthBrowserKeyBytes); err != nil {
			panic(fmt.Sprintf("generate OAuth browser ticket key: %v", err))
		}
	})
	return s.oauthBrowserKeyBytes
}

func (s *HelixAPIServer) signOAuthBrowserTicket(t oauthBrowserTicket) (string, error) {
	body, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, s.oauthBrowserKey())
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *HelixAPIServer) verifyOAuthBrowserTicket(raw string, now time.Time) (*oauthBrowserTicket, error) {
	payload, sig, ok := strings.Cut(raw, ".")
	if !ok {
		return nil, fmt.Errorf("malformed ticket")
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("malformed ticket")
	}
	mac := hmac.New(sha256.New, s.oauthBrowserKey())
	mac.Write([]byte(payload))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return nil, fmt.Errorf("invalid ticket")
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("malformed ticket")
	}
	var t oauthBrowserTicket
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("malformed ticket")
	}
	if now.Unix() > t.Expires {
		return nil, fmt.Errorf("expired ticket")
	}
	return &t, nil
}

// handleCreateOAuthBrowserTicket godoc
// @Summary Start an OAuth connection that a popup can finish without a Helix session
// @Description Starts the OAuth flow for the caller and returns a single-flow link, valid for two minutes, that binds the browser opening it to the flow and redirects to the provider. For apps that embed Helix and hold the person's own API key.
// @Tags oauth
// @Produce json
// @Param provider_id path string true "Provider ID"
// @Param scopes query string false "Comma-separated scopes"
// @Success 200 {object} map[string]string
// @Router /api/v1/oauth/flow/browser-ticket/{provider_id} [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleCreateOAuthBrowserTicket(_ http.ResponseWriter, r *http.Request) (map[string]string, error) {
	user := getRequestUser(r)
	if isAgentScopedKey(user) {
		// An agent must not link accounts to the person it runs for.
		return nil, system.NewHTTPError403("agent keys cannot connect accounts")
	}
	providerID := mux.Vars(r)["provider_id"]
	var scopes []string
	if raw := r.URL.Query().Get("scopes"); raw != "" {
		for _, sc := range strings.Split(raw, ",") {
			if sc = strings.TrimSpace(sc); sc != "" {
				scopes = append(scopes, sc)
			}
		}
	}
	authURL, err := s.oauthManager.StartOAuthFlow(r.Context(), user.ID, providerID, "", "", scopes)
	if err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("start OAuth flow: %v", err))
	}
	parsed, err := url.Parse(authURL)
	if err != nil || parsed.Query().Get("state") == "" {
		return nil, system.NewHTTPError500("the provider's authorization URL carries no state")
	}
	ticket, err := s.signOAuthBrowserTicket(oauthBrowserTicket{
		AuthURL: authURL, State: parsed.Query().Get("state"), Expires: time.Now().Add(oauthBrowserTicketTTL).Unix(),
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return map[string]string{
		"url": strings.TrimRight(s.Cfg.WebServer.URL, "/") + "/api/v1/oauth/flow/browser/" + ticket,
	}, nil
}

// oauthBrowserFlowHandler opens a browser ticket: it binds this browser to the
// ticket's flow and sends it to the provider. Public; the signed ticket is the
// credential.
func (s *HelixAPIServer) oauthBrowserFlowHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := s.verifyOAuthBrowserTicket(mux.Vars(r)["ticket"], time.Now())
		if err != nil {
			writeOAuthFailurePage(w, http.StatusBadRequest, oauthErrorInfo{
				Title:   "Link expired",
				Message: "This connection link is no longer valid. Close this window and click Connect again.",
				Color:   "#FC3600",
			})
			return
		}
		// The flow must still be pending: a used or expired state cannot be
		// resumed, and its cookie would bind nothing.
		tokens, err := s.Store.GetOAuthRequestTokenByState(r.Context(), t.State)
		if err != nil || len(tokens) == 0 || time.Now().After(tokens[0].ExpiresAt) {
			writeOAuthFailurePage(w, http.StatusBadRequest, oauthErrorInfo{
				Title:   "Link expired",
				Message: "This connection has already finished or timed out. Close this window and click Connect again.",
				Color:   "#FC3600",
			})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     oauthBrowserFlowCookie,
			Value:    t.State,
			Path:     "/api/v1/oauth/flow/callback",
			MaxAge:   int(time.Until(tokens[0].ExpiresAt).Seconds()),
			HttpOnly: true,
			Secure:   strings.HasPrefix(s.Cfg.WebServer.URL, "https://"),
			// The provider's redirect back is a top-level GET: Lax sends it.
			SameSite: http.SameSiteLaxMode,
		})
		log.Info().Str("user_id", tokens[0].UserID).Str("provider_id", tokens[0].ProviderID).Msg("OAuth browser ticket opened")
		http.Redirect(w, r, t.AuthURL, http.StatusFound)
	}
}

// oauthBrowserBound reports whether this browser opened a ticket for the flow
// named by state (oauthBrowserFlowHandler's cookie).
func oauthBrowserBound(r *http.Request, state string) bool {
	c, err := r.Cookie(oauthBrowserFlowCookie)
	if err != nil || c.Value == "" || state == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) == 1
}
