package apps

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

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
}

type GithubApp struct {
	Name         string
	Owner        string
	Repo         string
	GithubConfig config.GitHub
	helixConfig  *types.AppHelixConfig
	Client       *github.GithubClient
	App          *types.App
}

var HELIX_YAML_FILENAMES = []string{"helix.yaml", "helix.yml"}
var HELIX_DEPLOY_KEY_NAME = "helix-deploy-key"

func NewGithubApp(options GithubAppOptions) (*GithubApp, error) {
	parts := strings.Split(options.App.Config.Github.Repo, "/")
	if len(parts) != 2 {
		return nil, system.NewHTTPError500("invalid repo name")
	}
	return &GithubApp{
		Name:         options.App.Config.Github.Repo,
		Owner:        parts[0],
		Repo:         parts[1],
		GithubConfig: options.GithubConfig,
		Client:       options.Client,
		App:          options.App,
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

	// add the webhook to the repo
	err = githubApp.Client.AddWebhookToRepo(
		githubApp.Owner,
		githubApp.Repo,
		"helixwebhook",
		fmt.Sprintf("%s?repo=%s", githubApp.GithubConfig.WebhookURL, githubApp.Name),
		[]string{"push", "pull_request"},
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

	if config == nil {
		return nil, fmt.Errorf("helix.yaml not found in repo")
	}

	app.Config.Helix = config

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

func (githubApp *GithubApp) Update() (*types.App, error) {
	return githubApp.App, nil
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
	if config.GPTScripts.Scripts == nil {
		config.GPTScripts.Scripts = []types.AppHelixConfigGPTScript{}
	}

	newScripts := []types.AppHelixConfigGPTScript{}

	for i, script := range config.GPTScripts.Scripts {
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
		newScripts = append(newScripts, script)
	}

	config.GPTScripts.Scripts = newScripts

	if config.GPTScripts.Folder != "" {
		newScripts = []types.AppHelixConfigGPTScript{}

		if _, err := os.Stat(githubApp.Filepath(config.GPTScripts.Folder)); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("gpt scripts folder not found: %s", config.GPTScripts.Folder)
			}
			return nil, err
		}

		// look into the folder and extract the files script content
		err := filepath.Walk(githubApp.Filepath(config.GPTScripts.Folder), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				// if path doesn't end with ".gpt" then dont add
				if !strings.HasSuffix(path, ".gpt") {
					return nil
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				newScripts = append(newScripts, types.AppHelixConfigGPTScript{
					Name:    strings.TrimSuffix(filepath.Base(path), ".gpt"),
					Content: string(content),
				})
			}
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return config, nil
}
