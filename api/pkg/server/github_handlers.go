package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/go-github/github"
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

type GithubReposResponse struct {
	RedirectURL string               `json:"RedirectURL"`
	Repos       []*github.Repository `json:"Repos"`
}

func (apiServer *HelixAPIServer) listGithubRepos(res http.ResponseWriter, req *http.Request) ([]*github.Repository, error) {
	userContext := apiServer.getRequestContext(req)
	databaseToken, err := apiServer.getGithubDatabaseToken(userContext)
	if err != nil {
		return nil, err
	}
	if databaseToken == "" {
		return nil, fmt.Errorf("no github token found")
	}

	// use our token to get the list of repos from the github api
	client := github.NewClient(oauth2.NewClient(
		userContext.Ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: databaseToken,
			},
		),
	))
	repos := []*github.Repository{}
	opts := github.ListOptions{
		PerPage: 100,
		Page:    0,
	}
	for {
		result, meta, err := client.Repositories.List(context.Background(), "", &github.RepositoryListOptions{
			ListOptions: opts,
		})
		if err != nil {
			return nil, err
		}
		for _, repo := range result {
			if repo != nil {
				repos = append(repos, repo)
			}
		}
		opts.Page = opts.Page + 1
		if opts.Page > meta.LastPage {
			break
		}
	}
	return repos, nil
}

// func (apiServer *HelixAPIServer) githubEcdsaKeypair(w http.ResponseWriter, req *http.Request) {
// 	generate a new keypair
// 	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
// 	if err != nil {
// 		log.Println("Error generating keypair:", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	// convert to pem
// 	pkey, err := x509.MarshalPKCS8PrivateKey(privateKey)
// 	if err != nil {
// 		log.Println("Error marshalling keypair:", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	privateKeyPem := pem.EncodeToMemory(&pem.Block{
// 		Type:  "PRIVATE KEY",
// 		Bytes: pkey,
// 	})
// 	sshPubkey, err := ssh.NewPublicKey(&privateKey.PublicKey)
// 	if err != nil {
// 		log.Println("Error converting public key:", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	public := bytes.TrimSuffix(ssh.MarshalAuthorizedKey(sshPubkey), []byte{'\n'})
// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(map[string]string{
// 		"privateKey": string(privateKeyPem),
// 		"publicKey":  string(public),
// 	})
// }

// type GithubDeployKeyRequest struct {
// 	Owner     string `json:"Owner"`
// 	Repo      string `json:"Repo"`
// 	PublicKey string `json:"PublicKey"`
// }

// func (apiServer *HelixAPIServer) githubDeployKey(w http.ResponseWriter, req *http.Request) {
// 	requestData := GithubDeployKeyRequest{}
// 	err := json.NewDecoder(req.Body).Decode(&requestData)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}
// 	tokenResponse, err := ensureGithubToken(req)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}
// 	if !tokenResponse.HasToken {
// 		if err != nil {
// 			http.Error(w, "no access token found", http.StatusBadRequest)
// 			return
// 		}
// 	} else {
// 		client := github.NewClient(oauth2.NewClient(
// 			oauth2.NoContext,
// 			oauth2.StaticTokenSource(
// 				&oauth2.Token{AccessToken: tokenResponse.Token},
// 			),
// 		))
// 		keyTitle := "helix-deploy-key"
// 		key, _, err := client.Repositories.CreateKey(context.Background(), requestData.Owner, requestData.Repo, &github.Key{
// 			Key:   &requestData.PublicKey,
// 			Title: &keyTitle,
// 		})
// 		if err != nil {
// 			http.Error(w, err.Error(), http.StatusBadRequest)
// 			return
// 		}
// 		jsonResp, err := json.Marshal(key)
// 		if err != nil {
// 			http.Error(w, err.Error(), http.StatusBadRequest)
// 			return
// 		}
// 		w.Write(jsonResp)
// 	}
// }
