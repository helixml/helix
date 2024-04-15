package apps

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"
)

type GithubAppOptions struct {
	GithubConfig config.GitHub
	Client       *github.GithubClient
	App          *types.App
	UpdateApp    func(app *types.App) (*types.App, error)
}

type GithubApp struct {
	Name         string
	Owner        string
	Repo         string
	GithubConfig config.GitHub
	helixConfig  *types.AppHelixConfig
	Client       *github.GithubClient
	App          *types.App
	UpdateApp    func(app *types.App) (*types.App, error)
}

var HELIX_YAML_FILENAMES = []string{"helix.yaml", "helix.yml"}
var HELIX_DEPLOY_KEY_NAME = "helix-deploy-key"

func NewGithubApp(options GithubAppOptions) (*GithubApp, error) {
	parts := strings.Split(options.App.Config.Github.Repo, "/")
	if len(parts) != 2 {
		return nil, system.NewHTTPError400("invalid repo name")
	}
	return &GithubApp{
		Name:         options.App.Config.Github.Repo,
		Owner:        parts[0],
		Repo:         parts[1],
		GithubConfig: options.GithubConfig,
		Client:       options.Client,
		App:          options.App,
		UpdateApp:    options.UpdateApp,
	}, nil
}

// we get the repo setup to run as an app, this means
//   - create a keypair and assign to the provided repo
//   - upload the public key to the repo as a deploy key
//   - clone the repo
//   - checking for the location of a helix.yaml file
//   - checking the validity of the helix.yaml file
//   - assign the content of the helix.yaml file to the app config
func (githubApp *GithubApp) Create() (*types.App, error) {
	// check the repo exists
	_, err := githubApp.Client.GetRepo(githubApp.Owner, githubApp.Repo)
	if err != nil {
		return nil, err
	}
	// generate a keypair - we clone with SSH so that private repos
	// can also be connected
	keyPair, err := system.GenerateEcdsaKeypair()
	if err != nil {
		return nil, err
	}

	app := githubApp.App

	// assign the keypair to the config
	app.Config.Github.KeyPair = *keyPair

	// assign the public key to the repo
	err = githubApp.Client.AddPublicKeyToRepo(githubApp.Owner, githubApp.Repo, keyPair.PublicKey, HELIX_DEPLOY_KEY_NAME)
	if err != nil {
		return nil, err
	}

	// clone the repo locally
	err = githubApp.Clone()
	if err != nil {
		return nil, err
	}

	commitHash, err := github.GetRepoHash(githubApp.Filepath(""))
	if err != nil {
		return nil, err
	}

	app.Config.Github.Hash = commitHash

	webhookSigningSecret, err := system.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	app.Config.Github.WebhookSecret = webhookSigningSecret

	// we need to save the app at this point so when github tests the webhook
	// we have saved the signing secret into the database
	app, err = githubApp.UpdateApp(app)
	if err != nil {
		return nil, err
	}

	// add the webhook to the repo
	err = githubApp.Client.AddWebhookToRepo(
		githubApp.Owner,
		githubApp.Repo,
		"helixwebhook",
		fmt.Sprintf("%s?app_id=%s", githubApp.GithubConfig.WebhookURL, githubApp.App.ID),
		[]string{"push", "pull_request"},
		webhookSigningSecret,
	)
	if err != nil {
		return nil, err
	}

	config, err := githubApp.GetConfig()
	if err != nil {
		return nil, err
	}

	if config == nil {
		return nil, fmt.Errorf("helix.yaml not found in repo")
	}

	config, err = githubApp.processConfig(config)
	if err != nil {
		return nil, err
	}

	app.Config.Helix = config

	return app, nil
}

// this is called when github is pushed
func (githubApp *GithubApp) Update() (*types.App, error) {
	app := githubApp.App

	// update the code locally
	err := githubApp.Clone()
	if err != nil {
		return nil, err
	}

	commitHash, err := github.GetRepoHash(githubApp.Filepath(""))
	if err != nil {
		return nil, err
	}

	app.Updated = time.Now()
	app.Config.Github.Hash = commitHash

	config, err := githubApp.GetConfig()
	if err != nil {
		return nil, err
	}

	if config == nil {
		return nil, fmt.Errorf("helix.yaml not found in repo")
	}

	config, err = githubApp.processConfig(config)
	if err != nil {
		return nil, err
	}

	app.Config.Helix = config

	app, err = githubApp.UpdateApp(app)
	if err != nil {
		return nil, err
	}

	return app, nil
}

func (githubApp *GithubApp) Clone() error {
	return github.CloneOrUpdateRepo(
		// the name of the repo
		fmt.Sprintf("%s/%s", githubApp.Owner, githubApp.Repo),
		// the keypair for this app repo
		githubApp.App.Config.Github.KeyPair,
		// return the folder in which we should clone the repo
		githubApp.Filepath(""),
	)
}

func (githubApp *GithubApp) Filepath(subpath string) string {
	return path.Join(githubApp.GithubConfig.RepoFolder, githubApp.Name, subpath)
}

func (githubApp *GithubApp) RelativePath(fullpath string) string {
	return strings.TrimPrefix(fullpath, githubApp.Filepath(""))
}

func (githubApp *GithubApp) GetConfig() (*types.AppHelixConfig, error) {
	if githubApp.helixConfig != nil {
		return githubApp.helixConfig, nil
	}
	err := githubApp.Clone()
	if err != nil {
		return nil, err
	}
	for _, filename := range HELIX_YAML_FILENAMES {
		filepath := githubApp.Filepath(filename)
		if _, err := os.Stat(filepath); err != nil {
			if os.IsNotExist(err) {
				continue
			} else {
				return nil, err
			}
		} else {
			content, err := os.ReadFile(filepath)
			if err != nil {
				return nil, err
			}
			config := &types.AppHelixConfig{}
			err = yaml.Unmarshal(content, config)
			if err != nil {
				return nil, err
			}
			githubApp.helixConfig = config
			return config, nil
		}
	}
	return nil, fmt.Errorf("helix.yaml not found in repo")
}

func (githubApp *GithubApp) processConfig(config *types.AppHelixConfig) (*types.AppHelixConfig, error) {
	if config.GPTScript.Scripts == nil {
		config.GPTScript.Scripts = []types.AppHelixConfigGPTScript{}
	}

	if config.GPTScript.Files == nil {
		config.GPTScript.Files = []string{}
	}

	newScripts := []types.AppHelixConfigGPTScript{}

	// scripts means you can configure the GPTScript contents inline with the helix.yaml
	for i, script := range config.GPTScript.Scripts {
		if script.Name == "" {
			return nil, fmt.Errorf("gpt script %d has no name", i)
		}
		if script.FilePath != "" {
			if _, err := os.Stat(githubApp.Filepath(script.FilePath)); err != nil {
				if os.IsNotExist(err) {
					return nil, fmt.Errorf("gpt script not found: %s", script.FilePath)
				}
				return nil, err
			}

			content, err := os.ReadFile(githubApp.Filepath(script.FilePath))
			if err != nil {
				return nil, err
			}

			script.Content = string(content)
		}
		if script.Content == "" {
			return nil, fmt.Errorf("gpt script %s has no content", script.Name)
		}
		newScripts = append(newScripts, script)
	}

	expandedFiles, err := system.ExpandAndCheckFiles(githubApp.Filepath(""), config.GPTScript.Files)
	if err != nil {
		return nil, err
	}

	for _, filepath := range expandedFiles {
		content, err := os.ReadFile(filepath)
		if err != nil {
			return nil, err
		}
		newScripts = append(newScripts, types.AppHelixConfigGPTScript{
			Name:     path.Base(filepath),
			FilePath: githubApp.RelativePath(filepath),
			Content:  string(content),
		})
	}

	config.GPTScript.Scripts = newScripts

	return config, nil
}
