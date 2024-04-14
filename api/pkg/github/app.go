package github

import (
	"fmt"
	"path"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type GithubAppOptions struct {
	Config config.GitHub
	Client *GithubClient
	App    *types.App
}

type GithubApp struct {
	Name   string
	Owner  string
	Repo   string
	Config config.GitHub
	Client *GithubClient
	App    *types.App
}

var HELIX_YAML_FILENAMES = []string{"helix.yaml", "helix.yml"}

func NewGithubApp(options GithubAppOptions) (*GithubApp, error) {
	parts := strings.Split(options.App.Config.Github.Repo, "/")
	if len(parts) != 2 {
		return nil, system.NewHTTPError500("invalid repo name")
	}
	return &GithubApp{
		Name:   options.App.Config.Github.Repo,
		Owner:  parts[0],
		Repo:   parts[1],
		Config: options.Config,
		Client: options.Client,
		App:    options.App,
	}, nil
}

// we get the repo setup to run as an app, this means
//   - create a keypair and assign to the provided repo
//   - upload the public key to the repo as a deploy key
//   - clone the repo
//   - checking for the location of a helix.yaml file
//   - checking the validity of the helix.yaml file
//   - assign the content of the helix.yaml file to the app config
func (githubApp *GithubApp) Initialise() error {
	// check the repo exists
	_, err := githubApp.Client.GetRepo(githubApp.Owner, githubApp.Repo)
	if err != nil {
		return err
	}
	// generate a keypair - we clone with SSH so that private repos
	// can also be connected
	keyPair, err := system.GenerateEcdsaKeypair()
	if err != nil {
		return err
	}
	// assign the keypair to the config
	githubApp.App.Config.Github.KeyPair = *keyPair
	// assign the public key to the repo
	err = githubApp.Client.AddPublicKeyToRepo(githubApp.Owner, githubApp.Repo, keyPair.PublicKey, HELIX_DEPLOY_KEY_NAME)
	if err != nil {
		return err
	}
	// clone the repo locally
	err = githubApp.Clone()
	if err != nil {
		return err
	}

	err = githubApp.Client.AddWebhookToRepo(
		githubApp.Owner,
		githubApp.Repo,
		"helixwebhook",
		fmt.Sprintf("%s?repo=%s", githubApp.Config.WebhookURL, githubApp.Name),
		[]string{"push", "pull_request"},
	)
	if err != nil {
		return err
	}

	return nil
}

func (githubApp *GithubApp) Filepath(subpath string) string {
	return path.Join(githubApp.Config.RepoFolder, githubApp.Name, subpath)
}

func (githubApp *GithubApp) Clone() error {
	return cloneOrUpdateRepo(
		// the name of the repo
		fmt.Sprintf("%s/%s", githubApp.Owner, githubApp.Repo),
		// the keypair for this app repo
		githubApp.App.Config.Github.KeyPair,
		// return the folder in which we should clone the repo
		githubApp.Filepath(""),
	)
}
