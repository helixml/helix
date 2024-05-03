package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"

	// github_sdk "github.com/google/go-github/v61/github"

	github_api "github.com/google/go-github/v61/github"
	"github.com/helixml/helix/api/pkg/apps"
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

// given a loaded app record from our database
// get the user context setup so we can have a fully connected
// github client app - this is used from github webhooks
// and client frontends where there is no user context from an auth token
func (apiServer *HelixAPIServer) getGithubApp(app *types.App) (*apps.GithubApp, error) {
	client, err := apiServer.getGithubClientFromUserContext(types.RequestContext{
		User: types.User{
			ID:   app.Owner,
			Type: app.OwnerType,
		},
	})
	if err != nil {
		return nil, err
	}
	githubApp, err := apps.NewGithubApp(apps.GithubAppOptions{
		GithubConfig: apiServer.Cfg.GitHub,
		Client:       client,
		App:          app,
		ToolsPlanner: apiServer.Controller.ToolsPlanner,
		UpdateApp: func(app *types.App) (*types.App, error) {
			return apiServer.Store.UpdateApp(context.Background(), app)
		},
	})
	if err != nil {
		return nil, err
	}
	return githubApp, nil
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

		// only accept pushes to master or main
		if *evt.Ref != "refs/heads/master" && *evt.Ref != "refs/heads/main" {
			log.Info().Msgf("ignoring push to branch: %s %s", *evt.Ref, *evt.Repo.HTMLURL)
			return
		}

		log.Info().Msgf("github repo push: %+v", evt)

		githubApp, err := apiServer.getGithubApp(app)
		if err != nil {
			log.Error().Msgf("error getting github app: %s", err.Error())
			http.Error(w, fmt.Sprintf("error getting github app: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		app, err = githubApp.Update()
		if err != nil {
			app.Config.Error = err.Error()
			apiServer.Store.UpdateApp(r.Context(), app)
			log.Error().Msgf("error updating github app: %s", err.Error())
			http.Error(w, fmt.Sprintf("error updating github app: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}
}

// do we already have the github token as an api key in the database?
func (apiServer *HelixAPIServer) getGithubDatabaseToken(userContext types.RequestContext) (string, error) {
	apiKeys, err := apiServer.Store.ListAPIKeys(userContext.Ctx, &store.ListApiKeysQuery{
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
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
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
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
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
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
			userContext.User.Token,
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

	userContext := getRequestContext(req)
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
			RedirectURL: conf.AuthCodeURL(userContext.User.Email, oauth2.AccessTypeOffline),
		}, nil
	}
}

func (apiServer *HelixAPIServer) githubCallback(w http.ResponseWriter, req *http.Request) {
	userContext := getRequestContext(req)
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
	return apiServer.getGithubClientFromUserContext(getRequestContext(req))
}

func (apiServer *HelixAPIServer) getGithubClientFromUserContext(userContext types.RequestContext) (*github.GithubClient, error) {
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
