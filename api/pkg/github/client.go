package github

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/go-github/v61/github"
	"golang.org/x/oauth2"
)

// sameEvents compares two event lists irrespective of order /
// duplicates — UpsertWebhook uses it to decide whether to PATCH
// an adopted hook with a fresh events whitelist.
func sameEvents(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ca := append([]string(nil), a...)
	cb := append([]string(nil), b...)
	sort.Strings(ca)
	sort.Strings(cb)
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
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

// loadReposMaxPages caps the number of pages LoadRepos walks. With
// the default PerPage=100 this fetches up to 1000 repos sorted by
// most-recent-push — enough to surface anything the operator is
// actively working on without unbounded API spend on heavyweight
// accounts (some users belong to thousands of repos).
const loadReposMaxPages = 10

// LoadRepos returns the full_name of every repo the authenticated
// token can see — personal + org repos, public + private — sorted
// by most-recently-pushed first. Hits `GET /user/repos` via
// go-github's ListByAuthenticatedUser; paginates with a hard cap
// at loadReposMaxPages so a 5k-repo account doesn't take minutes.
//
// Limitations: the dropdown is best-effort. Operators on accounts
// with very many repos should still be able to type any
// `owner/name` they have access to into the freeSolo input — the
// downstream UpsertWebhook call will succeed against any repo the
// token has admin rights on, even if it didn't appear in this
// list.
func (c *Client) LoadRepos() ([]string, error) {
	var repos []*github.Repository
	opts := github.ListOptions{PerPage: 100, Page: 1}
	for i := 0; i < loadReposMaxPages; i++ {
		result, meta, err := c.client.Repositories.ListByAuthenticatedUser(c.ctx, &github.RepositoryListByAuthenticatedUserOptions{
			// "pushed" surfaces repos the operator has been
			// actively working on first — most relevant for
			// hooking up an automation stream.
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

// WebhookSummary captures the operator-facing bits of a GitHub
// webhook returned by the REST API. ID is the hook's numeric
// identifier; HTMLURL is the deep-link to its edit page (e.g.
// https://github.com/<owner>/<name>/settings/hooks/<id>).
type WebhookSummary struct {
	ID      int64
	HTMLURL string
	Events  []string
}

// UpsertWebhook creates a webhook on the repo if one doesn't already
// point at `url`, or returns the existing one when it does. Returns
// a typed summary (id + edit-page URL + active events) so callers
// can store the id + link back to GitHub's webhook UI.
//
// Events of `["*"]` is GitHub's wildcard meaning "deliver all events"
// — pass that for "send me everything" mode.
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
		// Existing hook with matching URL. Patch up the bits that
		// commonly drift (content_type, events, secret, active)
		// before adopting — otherwise an operator who created the
		// hook by hand with `form` content type ends up sending
		// `payload=<json>` to a JSON-only handler and every
		// delivery 400s. EditHook is idempotent server-side.
		needsUpdate := false
		if hook.Config.ContentType == nil || *hook.Config.ContentType != jsonCT {
			needsUpdate = true
		}
		if !sameEvents(hook.Events, events) {
			needsUpdate = true
		}
		if hook.Active == nil || !*hook.Active {
			needsUpdate = true
		}
		if !needsUpdate {
			return summarizeHook(hook), nil
		}
		if hook.ID == nil {
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

func summarizeHook(h *github.Hook) WebhookSummary {
	if h == nil {
		return WebhookSummary{}
	}
	out := WebhookSummary{Events: h.Events}
	if h.ID != nil {
		out.ID = *h.ID
	}
	if h.URL != nil {
		// GitHub's `url` is the API URL
		// (https://api.github.com/repos/<o>/<r>/hooks/<id>); convert
		// to the HTML settings URL operators care about.
		out.HTMLURL = apiHookURLToHTMLURL(*h.URL)
	}
	return out
}

// apiHookURLToHTMLURL converts the GitHub API hook URL to the
// human-facing settings page. The API returns
// `https://api.github.com/repos/<owner>/<name>/hooks/<id>`; the
// edit page is `https://github.com/<owner>/<name>/settings/hooks/<id>`.
// Best-effort: returns the input verbatim when the URL doesn't match
// the API pattern (e.g. GHE custom domains).
func apiHookURLToHTMLURL(apiURL string) string {
	const apiPrefix = "https://api.github.com/repos/"
	if len(apiURL) <= len(apiPrefix) || apiURL[:len(apiPrefix)] != apiPrefix {
		return apiURL
	}
	rest := apiURL[len(apiPrefix):]
	// rest = "<owner>/<name>/hooks/<id>"
	// Replace "/hooks/" with "/settings/hooks/" and prefix
	// https://github.com/.
	for i := 0; i+len("/hooks/") <= len(rest); i++ {
		if rest[i:i+len("/hooks/")] == "/hooks/" {
			return "https://github.com/" + rest[:i] + "/settings/hooks/" + rest[i+len("/hooks/"):]
		}
	}
	return apiURL
}
