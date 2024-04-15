package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog/log"

	// github_sdk "github.com/google/go-github/v61/github"

	github_api "github.com/google/go-github/v61/github"
	"github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/oauth2"
	github_oauth "golang.org/x/oauth2/github"
	"gopkg.in/rjz/githubhook.v0"
)

type GithubStatus struct {
	HasToken    bool   `json:"has_token"`
	RedirectURL string `json:"redirect_url"`
}

func (apiServer *HelixAPIServer) githubWebhook(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		log.Error().Msgf("github webhook app_id is required: %s", r.URL.String())
		http.Error(w, "app_id is required", http.StatusBadRequest)
		return
	}

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		log.Error().Msgf("error loading app from ID: %s %s", appID, err.Error())
		http.Error(w, fmt.Sprintf("error loading app: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	hook, err := githubhook.Parse([]byte(app.Config.Github.WebhookSecret), r)
	if err != nil {
		log.Error().Msgf("error parsing webhook: %s", err.Error())
		http.Error(w, fmt.Sprintf("error parsing webhook: %s", err.Error()), http.StatusBadRequest)
		return
	}

	formValues, err := url.ParseQuery(string(hook.Payload))
	if err != nil {
		log.Error().Msgf("error parsing form URL encoded payload: %s", err.Error())
		http.Error(w, fmt.Sprintf("error parsing form URL encoded payload: %s", err.Error()), http.StatusBadRequest)
		return
	}

	payload := formValues.Get("payload")

	if payload == "" {
		log.Error().Msgf("github webhook payload is required")
		http.Error(w, "payload is required", http.StatusBadRequest)
		return
	}

	switch hook.Event {

	case "push":
		evt := github_api.PushEvent{}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			log.Error().Msgf("error parsing webhook: %s", err.Error())
			http.Error(w, fmt.Sprintf("error parsing webhook: %s", err.Error()), http.StatusBadRequest)
			return
		}

		fmt.Printf("evt --------------------------------------\n")
		spew.Dump(evt)
	}
}

// do we already have the github token as an api key in the database?
func (apiServer *HelixAPIServer) getGithubDatabaseToken(userContext types.RequestContext) (string, error) {
	apiKeys, err := apiServer.Store.ListAPIKeys(userContext.Ctx, &store.ListApiKeysQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	})
	if err != nil {
		return "", err
	}
	for _, apiKey := range apiKeys {
		if apiKey.Type == types.APIKeyType_Github {
			return apiKey.Key, nil
		}
	}
	return "", nil
}

func (apiServer *HelixAPIServer) setGithubDatabaseToken(userContext types.RequestContext, token string) error {
	apiKeys, err := apiServer.Store.ListAPIKeys(userContext.Ctx, &store.ListApiKeysQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	})
	if err != nil {
		return err
	}
	for _, apiKey := range apiKeys {
		if apiKey.Type == types.APIKeyType_Github {
			err = apiServer.Store.DeleteAPIKey(userContext.Ctx, apiKey.Key)
			if err != nil {
				return err
			}
		}
	}
	_, err = apiServer.Store.CreateAPIKey(userContext.Ctx, &types.APIKey{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
		Name:      "github-oauth",
		Key:       token,
		Type:      types.APIKeyType_Github,
	})

	if err != nil {
		return err
	}

	return nil
}

func (apiServer *HelixAPIServer) getGithubOauthConfig(userContext types.RequestContext, pageURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     apiServer.Cfg.GitHub.ClientID,
		ClientSecret: apiServer.Cfg.GitHub.ClientSecret,
		Scopes:       []string{"repo", "admin:repo_hook"},
		RedirectURL: fmt.Sprintf(
			// we include their access token in the callback URL
			// so it is authenticated
			"%s%s/github/callback?access_token=%s&pageURL=%s",
			apiServer.Cfg.WebServer.URL,
			API_PREFIX,
			userContext.Token,
			url.QueryEscape(pageURL),
		),
		Endpoint: github_oauth.Endpoint,
	}
}

func (apiServer *HelixAPIServer) githubStatus(res http.ResponseWriter, req *http.Request) (*GithubStatus, error) {
	if !apiServer.Cfg.GitHub.Enabled || apiServer.Cfg.GitHub.ClientID == "" || apiServer.Cfg.GitHub.ClientSecret == "" {
		return nil, fmt.Errorf("github integration is not enabled")
	}
	pageURL := req.URL.Query().Get("pageURL")
	if pageURL == "" {
		return nil, fmt.Errorf("pageURL is required")
	}

	userContext := apiServer.getRequestContext(req)
	databaseToken, err := apiServer.getGithubDatabaseToken(userContext)
	if err != nil {
		return nil, err
	}

	if databaseToken != "" {
		return &GithubStatus{
			HasToken: true,
		}, nil
	} else {
		conf := apiServer.getGithubOauthConfig(userContext, pageURL)
		return &GithubStatus{
			HasToken:    false,
			RedirectURL: conf.AuthCodeURL(userContext.Email, oauth2.AccessTypeOffline),
		}, nil
	}
}

func (apiServer *HelixAPIServer) githubCallback(w http.ResponseWriter, req *http.Request) {
	userContext := apiServer.getRequestContext(req)
	pageURL := req.URL.Query().Get("pageURL")
	code := req.URL.Query().Get("code")
	conf := apiServer.getGithubOauthConfig(userContext, pageURL)
	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("error exchanging code for token: %s", err.Error()), http.StatusBadRequest)
		return
	}
	err = apiServer.setGithubDatabaseToken(userContext, token.AccessToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, req, pageURL, http.StatusFound)
}

func (apiServer *HelixAPIServer) getGithubClientFromRequest(req *http.Request) (*github.GithubClient, error) {
	userContext := apiServer.getRequestContext(req)
	databaseToken, err := apiServer.getGithubDatabaseToken(userContext)
	if err != nil {
		return nil, err
	}
	if databaseToken == "" {
		return nil, fmt.Errorf("no github token found")
	}
	return github.NewGithubClient(github.GithubClientOptions{
		Ctx:   userContext.Ctx,
		Token: databaseToken,
	})
}

func (apiServer *HelixAPIServer) listGithubRepos(res http.ResponseWriter, req *http.Request) ([]string, error) {
	client, err := apiServer.getGithubClientFromRequest(req)
	if err != nil {
		return nil, err
	}
	return client.LoadRepos()
}
