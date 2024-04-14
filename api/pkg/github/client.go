package github

import (
	"context"

	"github.com/google/go-github/v61/github"
	"golang.org/x/oauth2"
)

type GithubClientOptions struct {
	Ctx   context.Context
	Token string
}

type GithubClient struct {
	ctx    context.Context
	client *github.Client
}

func NewGithubClient(options GithubClientOptions) (*GithubClient, error) {
	client := github.NewClient(oauth2.NewClient(
		options.Ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: options.Token,
			},
		),
	))
	return &GithubClient{
		ctx:    options.Ctx,
		client: client,
	}, nil
}

func (githubClient *GithubClient) LoadRepos() ([]string, error) {
	repos := []*github.Repository{}
	opts := github.ListOptions{
		PerPage: 100,
		Page:    0,
	}
	for {
		result, meta, err := githubClient.client.Repositories.List(githubClient.ctx, "", &github.RepositoryListOptions{
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

func (githubClient *GithubClient) GetRepo(owner string, repo string) (*github.Repository, error) {
	result, _, err := githubClient.client.Repositories.Get(githubClient.ctx, owner, repo)
	return result, err
}

func (githubClient *GithubClient) AddPublicKeyToRepo(
	owner string,
	repo string,
	publicKey string,
	keyTitle string,
) error {
	_, _, err := githubClient.client.Repositories.CreateKey(context.Background(), owner, repo, &github.Key{
		Key:   &publicKey,
		Title: &keyTitle,
	})
	return err
}

func (githubClient *GithubClient) AddWebhookToRepo(
	owner string,
	repo string,
	name string,
	url string,
	events []string,
) error {
	active := true
	json := "application/json"

	hooks, _, err := githubClient.client.Repositories.ListHooks(githubClient.ctx, owner, repo, nil)
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
	_, _, err = githubClient.client.Repositories.CreateHook(context.Background(), owner, repo, &github.Hook{
		Active: &active,
		Name:   &name,
		URL:    &url,
		Events: events,
		Config: &github.HookConfig{
			ContentType: &json,
			URL:         &url,
		},
	})
	return err
}
