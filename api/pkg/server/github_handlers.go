package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

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
func (apiServer *HelixAPIServer) getGithubApp(app *types.App) (*apps.App, error) {
	client, err := apiServer.getGithubClientFromUserContext(context.Background(), &types.User{
		ID:   app.Owner,
		Type: app.OwnerType,
	})
	if err != nil {
		return nil, err
	}
	githubApp, err := apps.NewGithubApp(apps.AppOptions{
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

		var hash string
		app, err = githubApp.Update()
		if err != nil {
			// in this case - the app itself exists but the config has an error
			// so we try to record what the error was with the config so the user can see that
			// if app is nil here then it's just an error trying to get the config
			if app != nil && app.Config.Github == nil {
				app.Config.Github = &types.AppGithubConfig{}
				// we expect the github app to have already set the hash and timestamp
				// of the pulled config - if the config had an error we end up here
				app.Config.Github.LastUpdate.Error = err.Error()
			}
			log.Error().Msgf("error updating github app: %s", err.Error())
			http.Error(w, fmt.Sprintf("error updating github app: %s", err.Error()), http.StatusInternalServerError)
		} else {
			if app.Config.Github == nil {
				app.Config.Github = &types.AppGithubConfig{}
			}
			app.Config.Github.LastUpdate = types.AppGithubConfigUpdate{
				Updated: time.Now(),
				Hash:    hash,
			}
		}

		if app != nil {
			if _, err := apiServer.Store.UpdateApp(r.Context(), app); err != nil {
				log.Error().Msgf("error storing github app update: %v", err)
				http.Error(w, fmt.Sprintf("error storing github app update: %v", err), http.StatusInternalServerError)
			}
		}
	}
}

// do we already have the github token as an api key in the database?
func (apiServer *HelixAPIServer) getGithubDatabaseToken(ctx context.Context, user *types.User) (string, error) {
	apiKeys, err := apiServer.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return "", err
	}
	for _, apiKey := range apiKeys {
		if apiKey.Type == types.APIkeytypeGithub {
			return apiKey.Key, nil
		}
	}
	return "", nil
}

func (apiServer *HelixAPIServer) setGithubDatabaseToken(ctx context.Context, user *types.User, token string) error {
	apiKeys, err := apiServer.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return err
	}
	for _, apiKey := range apiKeys {
		if apiKey.Type == types.APIkeytypeGithub {
			err = apiServer.Store.DeleteAPIKey(ctx, apiKey.Key)
			if err != nil {
				return err
			}
		}
	}
	_, err = apiServer.Store.CreateAPIKey(ctx, &types.APIKey{
		Owner:     user.ID,
		OwnerType: user.Type,
		Name:      "github-oauth",
		Key:       token,
		Type:      types.APIkeytypeGithub,
	})

	if err != nil {
		return err
	}

	return nil
}

func (apiServer *HelixAPIServer) getGithubOauthConfig(user *types.User, pageURL string) *oauth2.Config {
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
			url.QueryEscape(user.Token),
			url.QueryEscape(pageURL),
		),
		Endpoint: github_oauth.Endpoint,
	}
}

func (apiServer *HelixAPIServer) githubStatus(_ http.ResponseWriter, req *http.Request) (*GithubStatus, error) {
	if !apiServer.Cfg.GitHub.Enabled || apiServer.Cfg.GitHub.ClientID == "" || apiServer.Cfg.GitHub.ClientSecret == "" {
		return nil, fmt.Errorf("github integration is not enabled")
	}
	pageURL := req.URL.Query().Get("pageURL")
	if pageURL == "" {
		return nil, fmt.Errorf("pageURL is required")
	}

	ctx := req.Context()
	user := getRequestUser(req)

	databaseToken, err := apiServer.getGithubDatabaseToken(ctx, user)
	if err != nil {
		return nil, err
	}

	if databaseToken != "" {
		return &GithubStatus{
			HasToken: true,
		}, nil
	}
	conf := apiServer.getGithubOauthConfig(user, pageURL)
	return &GithubStatus{
		HasToken:    false,
		RedirectURL: conf.AuthCodeURL(user.Email, oauth2.AccessTypeOffline),
	}, nil
}

func (apiServer *HelixAPIServer) githubCallback(w http.ResponseWriter, req *http.Request) {

	ctx := req.Context()
	user := getRequestUser(req)

	pageURL := req.URL.Query().Get("pageURL")
	code := req.URL.Query().Get("code")
	conf := apiServer.getGithubOauthConfig(user, pageURL)
	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("error exchanging code for token: %s", err.Error()), http.StatusBadRequest)
		return
	}
	err = apiServer.setGithubDatabaseToken(ctx, user, token.AccessToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, req, pageURL, http.StatusFound)
}

func (apiServer *HelixAPIServer) getGithubClientFromRequest(req *http.Request) (*github.Client, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	return apiServer.getGithubClientFromUserContext(ctx, user)
}

func (apiServer *HelixAPIServer) getGithubClientFromUserContext(ctx context.Context, user *types.User) (*github.Client, error) {
	databaseToken, err := apiServer.getGithubDatabaseToken(ctx, user)
	if err != nil {
		return nil, err
	}
	if databaseToken == "" {
		return nil, fmt.Errorf("no github token found")
	}
	return github.NewGithubClient(github.ClientOptions{
		Ctx:   ctx,
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
