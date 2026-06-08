package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v61/github"
	"golang.org/x/oauth2"
)

// ErrAppNotFound means the GitHub App no longer exists (or its key is invalid)
// — GitHub rejected the app JWT with 401/404. Callers use this to clear a stale
// stored connection after the app is deleted on GitHub.
var ErrAppNotFound = errors.New("github app not found (deleted or invalid credentials)")

// buildAppTransport builds the app-level JWT transport (authenticating as the
// GitHub App itself, not an installation), pointed at GHES when baseURL is
// non-empty. Centralises the ghinstallation + GHES base-URL setup for the
// app-scoped calls in this package.
func buildAppTransport(appID int64, privateKey, baseURL string) (*ghinstallation.AppsTransport, error) {
	atr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, []byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("create app transport: %w", err)
	}
	if baseURL != "" {
		atr.BaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3"
	}
	return atr, nil
}

// ListAppInstallations lists every installation of a GitHub App, authenticating
// as the app (JWT signed with its private key). Used to reconcile an org's
// installation id after the user installs the app, without relying on a
// browser Setup-URL redirect. baseURL is empty for github.com or the GHES origin.
func ListAppInstallations(ctx context.Context, appID int64, privateKey, baseURL string) ([]*github.Installation, error) {
	atr, err := buildAppTransport(appID, privateKey, baseURL)
	if err != nil {
		return nil, err
	}
	client := github.NewClient(&http.Client{Transport: atr})
	if baseURL != "" {
		if ec, err := client.WithEnterpriseURLs(baseURL, baseURL); err == nil {
			client = ec
		}
	}
	insts, resp, err := client.Apps.ListInstallations(ctx, &github.ListOptions{PerPage: 100})
	if err != nil {
		// A valid app authenticates and returns 200 even with zero
		// installations, so 401/404 here means the app itself is gone.
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("list app installations: %w", err)
	}
	return insts, nil
}

type ClientOptions struct {
	Ctx     context.Context
	Token   string
	BaseURL string // For GitHub Enterprise instances (empty for github.com)
}

type Client struct {
	ctx    context.Context
	client *github.Client
}

func NewGithubClient(options ClientOptions) (*Client, error) {
	httpClient := oauth2.NewClient(
		options.Ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: options.Token,
			},
		),
	)

	var client *github.Client

	if options.BaseURL != "" {
		// GitHub Enterprise - use custom base URL
		// The upload URL is typically the same as the base URL for GHE
		var err error
		client, err = github.NewClient(httpClient).WithEnterpriseURLs(options.BaseURL, options.BaseURL)
		if err != nil {
			return nil, err
		}
	} else {
		// Standard github.com
		client = github.NewClient(httpClient)
	}

	return &Client{
		ctx:    options.Ctx,
		client: client,
	}, nil
}

const loadReposMaxPages = 10

// LoadRepos returns full_name for every repo the token can see,
// sorted by most-recently-pushed first. Caps at loadReposMaxPages so
// heavy accounts don't take minutes.
func (c *Client) LoadRepos() ([]string, error) {
	var repos []*github.Repository
	opts := github.ListOptions{PerPage: 100, Page: 1}
	for i := 0; i < loadReposMaxPages; i++ {
		result, meta, err := c.client.Repositories.ListByAuthenticatedUser(c.ctx, &github.RepositoryListByAuthenticatedUserOptions{
			Sort:        "pushed",
			Direction:   "desc",
			ListOptions: opts,
		})
		if err != nil {
			return nil, err
		}
		repos = append(repos, result...)
		if meta == nil || meta.NextPage == 0 {
			break
		}
		opts.Page = meta.NextPage
	}
	results := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repo != nil && repo.FullName != nil {
			results = append(results, *repo.FullName)
		}
	}
	return results, nil
}

// CompleteAppManifest exchanges the temporary code GitHub returns at the end
// of the App Manifest flow for the new app's permanent config: id, slug,
// PEM private key, webhook secret and client credentials. The code is valid
// for one hour and the call is unauthenticated (the code is the credential),
// so this uses a tokenless client. baseURL is empty for github.com or the
// GHES origin.
func CompleteAppManifest(ctx context.Context, code, baseURL string) (*github.AppConfig, error) {
	client := github.NewClient(nil)
	if baseURL != "" {
		enterpriseClient, err := client.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, fmt.Errorf("configure GHES base url: %w", err)
		}
		client = enterpriseClient
	}
	cfg, _, err := client.Apps.CompleteAppManifest(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("complete app manifest: %w", err)
	}
	return cfg, nil
}

// LoadInstallationRepos returns full_name for every repo a GitHub App
// installation can access, via GET /installation/repositories. Use this when
// the client is built with an installation access token (the app bot
// identity) rather than a user OAuth token — an installation token cannot
// call the /user/repos endpoint LoadRepos uses, and listing the
// installation's repos is exactly the right scope for the picker: you can
// only wire a stream to a repo the bot can actually act on.
func (c *Client) LoadInstallationRepos() ([]string, error) {
	var repos []*github.Repository
	opts := github.ListOptions{PerPage: 100, Page: 1}
	for i := 0; i < loadReposMaxPages; i++ {
		result, meta, err := c.client.Apps.ListRepos(c.ctx, &opts)
		if err != nil {
			return nil, err
		}
		if result != nil {
			repos = append(repos, result.Repositories...)
		}
		if meta == nil || meta.NextPage == 0 {
			break
		}
		opts.Page = meta.NextPage
	}
	results := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repo != nil && repo.FullName != nil {
			results = append(results, *repo.FullName)
		}
	}
	return results, nil
}

func (c *Client) GetRepo(owner string, repo string) (*github.Repository, error) {
	result, _, err := c.client.Repositories.Get(c.ctx, owner, repo)
	return result, err
}

func (c *Client) AddPublicKeyToRepo(
	owner string,
	repo string,
	publicKey string,
	keyTitle string,
) error {
	_, _, err := c.client.Repositories.CreateKey(context.Background(), owner, repo, &github.Key{
		Key:   &publicKey,
		Title: &keyTitle,
	})
	return err
}

func (c *Client) AddWebhookToRepo(
	owner string,
	repo string,
	name string,
	url string,
	events []string,
	secret string,
) error {
	_, err := c.UpsertWebhook(owner, repo, name, url, events, secret)
	return err
}

type WebhookSummary struct {
	ID     int64
	Events []string
}

// UpsertWebhook creates a webhook on the repo if none points at `url`,
// or PATCHes the existing one to bring content_type / events / active
// in line. Events of `["*"]` is GitHub's wildcard.
func (c *Client) UpsertWebhook(
	owner string,
	repo string,
	name string,
	url string,
	events []string,
	secret string,
) (WebhookSummary, error) {
	active := true
	jsonCT := "application/json"

	hooks, _, err := c.client.Repositories.ListHooks(c.ctx, owner, repo, nil)
	if err != nil {
		return WebhookSummary{}, err
	}
	for _, hook := range hooks {
		if hook.Config == nil || hook.Config.URL == nil || *hook.Config.URL != url {
			continue
		}
		needsUpdate := hook.Config.ContentType == nil || *hook.Config.ContentType != jsonCT ||
			!sameEvents(hook.Events, events) ||
			hook.Active == nil || !*hook.Active
		if !needsUpdate || hook.ID == nil {
			return summarizeHook(hook), nil
		}
		patched, _, err := c.client.Repositories.EditHook(context.Background(), owner, repo, *hook.ID, &github.Hook{
			Active: &active,
			Events: events,
			Config: &github.HookConfig{
				ContentType: &jsonCT,
				URL:         &url,
				Secret:      &secret,
			},
		})
		if err != nil {
			return WebhookSummary{}, fmt.Errorf("update github webhook: %w", err)
		}
		return summarizeHook(patched), nil
	}
	created, _, err := c.client.Repositories.CreateHook(context.Background(), owner, repo, &github.Hook{
		Active: &active,
		Name:   &name,
		URL:    &url,
		Events: events,
		Config: &github.HookConfig{
			ContentType: &jsonCT,
			URL:         &url,
			Secret:      &secret,
		},
	})
	if err != nil {
		return WebhookSummary{}, err
	}
	return summarizeHook(created), nil
}

// WebhookSettingsURL returns the operator-facing edit page for a hook.
func WebhookSettingsURL(owner, repo string, hookID int64) string {
	return fmt.Sprintf("https://github.com/%s/%s/settings/hooks/%d", owner, repo, hookID)
}

func sameEvents(a, b []string) bool {
	ca := slices.Clone(a)
	cb := slices.Clone(b)
	slices.Sort(ca)
	slices.Sort(cb)
	return slices.Equal(ca, cb)
}

func summarizeHook(h *github.Hook) WebhookSummary {
	if h == nil {
		return WebhookSummary{}
	}
	out := WebhookSummary{Events: h.Events}
	if h.ID != nil {
		out.ID = *h.ID
	}
	return out
}
