package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"
	github_oauth "golang.org/x/oauth2/github"
)

type GithubTokenResponse struct {
	HasToken    bool   `json:"HasToken"`
	Token       string `json:"Token"`
	RedirectURL string `json:"RedirectURL"`
}

func getGithubOauthConf(tenantId, userId, pageURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_INTEGRATION_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_INTEGRATION_CLIENT_SECRET"),
		Scopes:       []string{"repo"},
		RedirectURL:  fmt.Sprintf("%s?tenantId=%s&userId=%s&pageURL=%s", os.Getenv("GITHUB_INTEGRATION_CALLBACK_URL"), tenantId, userId, pageURL),
		Endpoint:     github_oauth.Endpoint,
	}
}

func ensureGithubToken(req *http.Request) (*GithubTokenResponse, error) {
	tenantId, err := mdw.tenantIdFromRequest(req)
	if err != nil {
		return nil, err
	}
	jwt, err := mdw.userFromRequest(req)
	if err != nil {
		return nil, err
	}
	userId := getUserIdFromJWT(jwt)

	// XXX SECURITY: all users of this tenant will see all other users
	// github tokens (because this config is shared by all users in a tenant)
	accessTokenKey := fmt.Sprintf("githubAccessToken_%s", userId)
	ensureConfigReconciler(tenantId)
	config := getInstallationConfig(tenantId)
	token, ok := config[accessTokenKey]

	if ok {
		return &GithubTokenResponse{
			HasToken: true,
			Token:    token.(string),
		}, nil
	} else {
		pageURL := req.URL.Query().Get("pageURL")
		conf := getGithubOauthConf(tenantId, userId, pageURL)
		return &GithubTokenResponse{
			HasToken: false,
			// XXX security: we should add an actual state param to prevent
			// MITM attacks: https://docs.github.com/en/developers/apps/building-oauth-apps/authorizing-oauth-apps#2-users-are-redirected-back-to-your-site-by-github
			RedirectURL: conf.AuthCodeURL("state", oauth2.AccessTypeOffline),
		}, nil
	}
}

// XXX SECURITY: a user can change the query params and mess with other
// XXX SECURITY: anyone can now trigger the callback so let's require
// a valid keycloak cookie here
// tenants and users recorded tokens here
// check the authorized users tenantID against the attempted tenantId

// router.Methods("GET").Path("/v1/github/callback").HandlerFunc(
func (apiServer *HelixAPIServer) githubCallback(w http.ResponseWriter, req *http.Request) {
	// read the tenantId and userId values from the query parameters of req
	tenantId := req.URL.Query().Get("tenantId")
	userId := req.URL.Query().Get("userId")
	pageURL := req.URL.Query().Get("pageURL")
	code := req.URL.Query().Get("code")

	conf := getGithubOauthConf(tenantId, userId, pageURL)

	token, err := conf.Exchange(oauth2.NoContext, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("error exchanging code for token: %s", err.Error()), http.StatusBadRequest)
		return
	}

	accessTokenKey := fmt.Sprintf("githubAccessToken_%s", userId)
	data := TenantInstallationConfig{}
	data[accessTokenKey] = token.AccessToken
	err = setInstallationConfig(tenantId, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("error exchanging code for token: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// redirect the response to redirectURL
	http.Redirect(w, req, pageURL, http.StatusFound)
}

type GithubReposResponse struct {
	RedirectURL string               `json:"RedirectURL"`
	Repos       []*github.Repository `json:"Repos"`
}

func (apiServer *HelixAPIServer) githubRepos(w http.ResponseWriter, req *http.Request) {
	tokenResponse, err := ensureGithubToken(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !tokenResponse.HasToken {
		jsonResp, err := json.Marshal(GithubReposResponse{
			RedirectURL: tokenResponse.RedirectURL,
		})
		if err != nil {
			log.Fatalf("Error happened in JSON marshal. Err: %s", err)
		}
		w.Write(jsonResp)
	} else {
		// use our token to get the list of repos from the github api
		client := github.NewClient(oauth2.NewClient(
			oauth2.NoContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: tokenResponse.Token},
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
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
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
		jsonResp, err := json.Marshal(GithubReposResponse{
			Repos: repos,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write(jsonResp)
	}
}

func (apiServer *HelixAPIServer) githubEcdsaKeypair(w http.ResponseWriter, req *http.Request) {
	// generate a new keypair
	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		log.Println("Error generating keypair:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// convert to pem
	pkey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		log.Println("Error marshalling keypair:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	privateKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkey,
	})
	sshPubkey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		log.Println("Error converting public key:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	public := bytes.TrimSuffix(ssh.MarshalAuthorizedKey(sshPubkey), []byte{'\n'})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"privateKey": string(privateKeyPem),
		"publicKey":  string(public),
	})
}

type GithubDeployKeyRequest struct {
	Owner     string `json:"Owner"`
	Repo      string `json:"Repo"`
	PublicKey string `json:"PublicKey"`
}

func (apiServer *HelixAPIServer) githubDeployKey(w http.ResponseWriter, req *http.Request) {
	requestData := GithubDeployKeyRequest{}
	err := json.NewDecoder(req.Body).Decode(&requestData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tokenResponse, err := ensureGithubToken(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !tokenResponse.HasToken {
		if err != nil {
			http.Error(w, "no access token found", http.StatusBadRequest)
			return
		}
	} else {
		client := github.NewClient(oauth2.NewClient(
			oauth2.NoContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: tokenResponse.Token},
			),
		))
		keyTitle := "helix-deploy-key"
		key, _, err := client.Repositories.CreateKey(context.Background(), requestData.Owner, requestData.Repo, &github.Key{
			Key:   &requestData.PublicKey,
			Title: &keyTitle,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonResp, err := json.Marshal(key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write(jsonResp)
	}
}
