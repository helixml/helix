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
	keyPair, err := system.GenerateEcdsaKeypair()
	if err != nil {
		return err
	}
	githubApp.App.Config.Github.KeyPair = *keyPair
	err = githubApp.Client.AddPublicKeyToRepo(githubApp.Owner, githubApp.Repo, keyPair.PublicKey, "helix-deploy-key")
	if err != nil {
		return err
	}
	err = githubApp.Clone()
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
		fmt.Sprintf("%s/%s", githubApp.Owner, githubApp.Repo),
		githubApp.App.Config.Github.KeyPair,
		githubApp.Filepath(""),
	)
}
