package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/oauth2"
	github_oauth "golang.org/x/oauth2/github"
)

type GithubStatus struct {
	HasToken    bool   `json:"has_token"`
	RedirectURL string `json:"redirect_url"`
}

// do we already have the github token as an api key in the database?
func (apiServer *HelixAPIServer) getGithubDatabaseToken(userContext types.RequestContext) (string, error) {
	apiKeys, err := apiServer.Store.GetAPIKeys(userContext.Ctx, store.OwnerQuery{
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
	apiKeys, err := apiServer.Store.GetAPIKeys(userContext.Ctx, store.OwnerQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	})
	if err != nil {
		return err
	}
	for _, apiKey := range apiKeys {
		if apiKey.Type == types.APIKeyType_Github {
			err = apiServer.Store.DeleteAPIKey(userContext.Ctx, *apiKey)
			if err != nil {
				return err
			}
		}
	}
	_, err = apiServer.Store.CreateAPIKey(userContext.Ctx, store.OwnerQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	}, "github-oauth", token, types.APIKeyType_Github)

	if err != nil {
		return err
	}

	return nil
}

func (apiServer *HelixAPIServer) getGithubOauthConfig(userContext types.RequestContext, pageURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     apiServer.Cfg.GitHub.ClientID,
		ClientSecret: apiServer.Cfg.GitHub.ClientSecret,
		Scopes:       []string{"repo"},
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
