package apps

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"
)

type GithubAppOptions struct {
	GithubConfig config.GitHub
	Client       *github.GithubClient
	App          *types.App
	ToolsPlanner tools.Planner
	UpdateApp    func(app *types.App) (*types.App, error)
}

type GithubApp struct {
	Name         string
	Owner        string
	Repo         string
	GithubConfig config.GitHub
	helixConfig  *types.AppHelixConfig
	Client       *github.GithubClient
	ToolsPlanner tools.Planner
	App          *types.App
	UpdateApp    func(app *types.App) (*types.App, error)
}

var HELIX_YAML_FILENAMES = []string{"helix.yaml", "helix.yml"}
var HELIX_DEPLOY_KEY_NAME = "helix-deploy-key"

func NewGithubApp(options GithubAppOptions) (*GithubApp, error) {
	parts := strings.Split(options.App.Config.Github.Repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo name")
	}
	if options.Client == nil {
		return nil, fmt.Errorf("Client is required")
	}
	if options.ToolsPlanner == nil {
		return nil, fmt.Errorf("ToolsPlanner is required")
	}
	if options.App == nil {
		return nil, fmt.Errorf("App struct is required")
	}
	if options.UpdateApp == nil {
		return nil, fmt.Errorf("UpdateApp function is required")
	}
	return &GithubApp{
		Name:         options.App.Config.Github.Repo,
		Owner:        parts[0],
		Repo:         parts[1],
		GithubConfig: options.GithubConfig,
		Client:       options.Client,
		ToolsPlanner: options.ToolsPlanner,
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

	app.Config.Helix = *config

	commitHash, err := github.GetRepoHash(githubApp.Filepath(""))
	if err != nil {
		return nil, err
	}

	app.Config.Github.Hash = commitHash
	app.Config.Github.LastUpdate = types.AppGithubConfigUpdate{
		Updated: time.Now(),
		Hash:    commitHash,
	}

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

	// do this here because then if there is an error we will mark
	// the latest update as an error but not update the app itself
	app.Config.Github.LastUpdate = types.AppGithubConfigUpdate{
		Updated: time.Now(),
		Hash:    commitHash,
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
		// if there is an error here it means there is a problem with the config
		// we have loaded from github - let's mark the latest update as an error
		// but not hoist the config up to the app to leave the last known working
		// version alone
		app.Config.Github.LastUpdate.Error = err.Error()

		// TODO: what is best practice here?
		// as in we are already in an error handling sitch
		// what do we do when there is another error?
		// the only reason we want to update the app right now is to let
		// the user know that there was an error processing their latest config
		// (I need someone who knows how to handle errors better than me to help me here)
		githubApp.UpdateApp(app)

		return nil, err
	}

	// do this after we have called processConfig
	// because then if there is a problem with the config
	// we won't actually update the app
	app.Config.Github.Hash = commitHash
	app.Config.Helix = *config

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
	if config.Assistants == nil {
		config.Assistants = []types.AssistantConfig{}
	}

	for i, assistant := range config.Assistants {
		if assistant.APIs == nil {
			config.Assistants[i].APIs = []types.AssistantAPI{}
		}
		if assistant.GPTScripts == nil {
			config.Assistants[i].GPTScripts = []types.AssistantGPTScript{}
		}

		newTools := []types.Tool{}
		newScripts := []types.AssistantGPTScript{}

		// scripts means you can configure the GPTScript contents inline with the helix.yaml
		for _, script := range assistant.GPTScripts {
			if script.File != "" {
				expandedFiles, err := system.ExpandAndCheckFiles(githubApp.Filepath(""), []string{script.File})
				if err != nil {
					return nil, err
				}
				for _, filepath := range expandedFiles {
					content, err := os.ReadFile(filepath)
					if err != nil {
						return nil, err
					}
					file := githubApp.RelativePath(filepath)
					newScripts = append(newScripts, types.AssistantGPTScript{
						Name:        file,
						File:        file,
						Content:     string(content),
						Description: script.Description,
					})
					newTools = append(newTools, types.Tool{
						ID:       system.GenerateToolID(),
						Name:     file,
						ToolType: types.ToolTypeGPTScript,
						Config: types.ToolConfig{
							GPTScript: &types.ToolGPTScriptConfig{
								Script: string(content),
							},
						},
						Created: time.Now(),
						Updated: time.Now(),
					})
				}
			} else {
				if script.Content == "" {
					return nil, fmt.Errorf("gpt script %s has no content", script.Name)
				}
				newScripts = append(newScripts, script)
				newTools = append(newTools, types.Tool{
					ID:          system.GenerateToolID(),
					Name:        script.Name,
					Description: script.Description,
					ToolType:    types.ToolTypeGPTScript,
					Config: types.ToolConfig{
						GPTScript: &types.ToolGPTScriptConfig{
							Script: script.Content,
						},
					},
					Created: time.Now(),
					Updated: time.Now(),
				})
			}
		}

		newAPIs := []types.AssistantAPI{}

		for _, api := range assistant.APIs {
			if api.Schema == "" {
				return nil, fmt.Errorf("api %s has no schema", api.Name)
			}

			processedSchema, err := githubApp.processApiSchema(api.Schema)
			if err != nil {
				return nil, err
			}

			api.Schema = processedSchema

			if api.Headers == nil {
				api.Headers = map[string]string{}
			}

			if api.Query == nil {
				api.Query = map[string]string{}
			}

			newTools = append(newTools, types.Tool{
				ID:          system.GenerateToolID(),
				Created:     time.Now(),
				Updated:     time.Now(),
				Name:        api.Name,
				Description: api.Description,
				ToolType:    types.ToolTypeAPI,
				Config: types.ToolConfig{
					API: &types.ToolApiConfig{
						URL:                     api.URL,
						Schema:                  api.Schema,
						Headers:                 api.Headers,
						Query:                   api.Query,
						RequestPrepTemplate:     api.RequestPrepTemplate,
						ResponseSuccessTemplate: api.ResponseSuccessTemplate,
						ResponseErrorTemplate:   api.ResponseErrorTemplate,
					},
				},
			})
		}

		for i := range newTools {
			err := tools.ValidateTool(&newTools[i], githubApp.ToolsPlanner, false)
			if err != nil {
				return nil, err
			}
		}

		config.Assistants[i].GPTScripts = newScripts
		config.Assistants[i].APIs = newAPIs
		config.Assistants[i].Tools = newTools
	}

	return config, nil
}

func (githubApp *GithubApp) processApiSchema(schema string) (string, error) {
	if strings.HasPrefix(strings.ToLower(schema), "http://") || strings.HasPrefix(strings.ToLower(schema), "https://") {
		client := system.NewRetryClient(3)
		resp, err := client.Get(schema)
		if err != nil {
			return "", fmt.Errorf("failed to get schema from URL: %w", err)
		}
		defer resp.Body.Close()
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}
		return string(bts), nil
	}

	// if the schema is only one line then assume it's a file path
	if !strings.Contains(schema, "\n") && !strings.Contains(schema, "\r") {
		// it must be a YAML file
		if !strings.HasSuffix(schema, ".yaml") && !strings.HasSuffix(schema, ".yml") {
			return "", fmt.Errorf("schema must be in yaml format")
		}
		content, err := os.ReadFile(githubApp.Filepath(schema))
		if err != nil {
			return "", fmt.Errorf("failed to read schema file: %w", err)
		}
		return string(content), nil
	}

	return schema, nil
}
