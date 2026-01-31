package github

import (
	"context"

	"github.com/google/go-github/v61/github"
	"golang.org/x/oauth2"
)

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

func (c *Client) LoadRepos() ([]string, error) {
	repos := []*github.Repository{}
	opts := github.ListOptions{
		PerPage: 100,
		Page:    0,
	}
	for {
		result, meta, err := c.client.Repositories.ListByUser(c.ctx, "", &github.RepositoryListByUserOptions{
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
	results := []string{}
	for _, repo := range repos {
		results = append(results, *repo.FullName)
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
	active := true
	json := "application/json"

	hooks, _, err := c.client.Repositories.ListHooks(c.ctx, owner, repo, nil)
	if err != nil {
		return err
	}

	for _, hook := range hooks {
		if hook.Config.URL != nil && *hook.Config.URL == url {
			// Hook already exists, no need to add it again
			return nil
		}
	}

	// Add the new hook
	_, _, err = c.client.Repositories.CreateHook(context.Background(), owner, repo, &github.Hook{
		Active: &active,
		Name:   &name,
		URL:    &url,
		Events: events,
		Config: &github.HookConfig{
			ContentType: &json,
			URL:         &url,
			Secret:      &secret,
		},
	})
	return err
}
