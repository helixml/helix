package controller

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

//go:embed filestore_folders.json
var jsonFile embed.FS

func GetFolders() ([]filestore.Folder, error) {
	file, err := jsonFile.Open("filestore_folders.json")
	if err != nil {
		return []filestore.Folder{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return []filestore.Folder{}, err
	}

	var folders []filestore.Folder
	if err := json.Unmarshal(content, &folders); err != nil {
		return []filestore.Folder{}, err
	}

	return folders, nil
}

func GetSessionFolder(sessionID string) string {
	return filepath.Join("sessions", sessionID)
}

func GetDataEntityFolder(ID string) string {
	return filepath.Join("data", ID)
}

func GetInteractionInputsFolder(sessionID string, interactionID string) string {
	return filepath.Join(GetSessionFolder(sessionID), "inputs", interactionID)
}

func GetSessionResultsFolder(sessionID string) string {
	return filepath.Join(GetSessionFolder(sessionID), "results")
}

func (c *Controller) GetFilestoreUserPath(ctx types.OwnerContext, path string) (string, error) {
	userPrefix := filestore.GetUserPrefix(c.Options.Config.Controller.FilePrefixGlobal, ctx.Owner)

	return filepath.Join(userPrefix, path), nil
}

// GetFilestoreAppPath returns a path scoped to the app's directory
// This uses the app-scoped structure: {filestorePrefix}/apps/{appID}/{path}
func (c *Controller) GetFilestoreAppPath(appID, path string) (string, error) {
	appPrefix := filestore.GetAppPrefix(c.Options.Config.Controller.FilePrefixGlobal, appID)

	return filepath.Join(appPrefix, path), nil
}

// GetFilestoreAppKnowledgePath returns a path scoped to the app's knowledge directory
// This ensures that knowledge paths are always within the app's directory
func (c *Controller) GetFilestoreAppKnowledgePath(ctx types.OwnerContext, appID, knowledgePath string) (string, error) {
	log.Debug().
		Str("app_id", appID).
		Str("knowledge_path", knowledgePath).
		Msgf("Getting filestore app knowledge path")

	// Use the new app prefix structure directly
	appPrefix := filestore.GetAppPrefix(c.Options.Config.Controller.FilePrefixGlobal, appID)

	// If the path already starts with apps/:app_id, we'll strip it to avoid duplication
	appPathCheck := filepath.Join("apps", appID)
	if strings.HasPrefix(knowledgePath, appPathCheck) {
		originalPath := knowledgePath
		knowledgePath = knowledgePath[len(appPathCheck):]
		// Remove any leading slashes
		knowledgePath = strings.TrimPrefix(knowledgePath, "/")

		log.Debug().
			Str("app_id", appID).
			Str("original_path", originalPath).
			Str("stripped_path", knowledgePath).
			Msgf("Stripped app prefix from path")
	}

	finalPath := filepath.Join(appPrefix, knowledgePath)

	log.Debug().
		Str("app_id", appID).
		Str("original_path", knowledgePath).
		Str("app_prefix", appPrefix).
		Str("final_path", finalPath).
		Msgf("Constructed final filestore path")

	return finalPath, nil
}

func (c *Controller) VerifySignature(url string) bool {
	return filestore.VerifySignature(url, c.Options.Config.Controller.FilestorePresignSecret)
}

func (c *Controller) GetFilestoreSessionPath(ctx types.OwnerContext, sessionID string) (string, error) {
	return c.GetFilestoreUserPath(ctx, GetSessionFolder(sessionID))
}

func (c *Controller) GetFilestoreInteractionInputsPath(ctx types.OwnerContext, sessionID string, interactionID string) (string, error) {
	return c.GetFilestoreUserPath(ctx, GetInteractionInputsFolder(sessionID, interactionID))
}

func (c *Controller) GetFilestoreResultsPath(ctx types.OwnerContext, sessionID string, _ string) (string, error) {
	return c.GetFilestoreUserPath(ctx, GetSessionResultsFolder(sessionID))
}

// given a path - we might have never seen this filestore yet
// so, we must ensure that the users base path is created and then create each
// special folder as listed above
func (c *Controller) ensureFilestoreUserPath(ctx types.OwnerContext, path string) (string, error) {
	userPath, err := c.GetFilestoreUserPath(ctx, "")
	if err != nil {
		return "", err
	}
	_, err = c.Options.Filestore.CreateFolder(c.Ctx, userPath)
	if err != nil {
		return "", err
	}

	// now we loop over the top level folders and ensure they exist
	folders, err := GetFolders()
	if err != nil {
		return "", err
	}
	for _, folder := range folders {
		_, err := c.Options.Filestore.CreateFolder(c.Ctx, filepath.Join(userPath, folder.Name))
		if err != nil {
			return "", err
		}
	}
	retPath, err := c.GetFilestoreUserPath(ctx, path)
	if err != nil {
		return "", err
	}
	return retPath, nil
}

func (c *Controller) FilestoreConfig(ctx types.OwnerContext) (filestore.Config, error) {
	userPrefix := filestore.GetUserPrefix(c.Options.Config.Controller.FilePrefixGlobal, ctx.Owner)

	folders, err := GetFolders()
	if err != nil {
		return filestore.Config{}, err
	}
	return filestore.Config{
		UserPrefix: userPrefix,
		Folders:    folders,
	}, nil
}

func (c *Controller) FilestoreList(ctx types.OwnerContext, path string) ([]filestore.Item, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.List(c.Ctx, filePath)
}

func (c *Controller) FilestoreGet(ctx types.OwnerContext, path string) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.Get(c.Ctx, filePath)
}

func (c *Controller) FilestoreCreateFolder(ctx types.OwnerContext, path string) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.CreateFolder(c.Ctx, filePath)
}

func (c *Controller) FilestoreDownloadFile(ctx types.OwnerContext, path string) (io.ReadCloser, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.OpenFile(c.Ctx, filePath)
}

func (c *Controller) FilestoreDownloadFolder(ctx types.OwnerContext, path string) (io.Reader, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.DownloadFolder(c.Ctx, filePath)
}

func (c *Controller) FilestoreUploadFile(ctx types.OwnerContext, path string, r io.Reader) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.WriteFile(c.Ctx, filePath, r)
}

func (c *Controller) FilestoreRename(ctx types.OwnerContext, path string, newPath string) (filestore.Item, error) {
	fullPath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.Item{}, err
	}
	fullNewPath, err := c.ensureFilestoreUserPath(ctx, newPath)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.Rename(c.Ctx, fullPath, fullNewPath)
}

func (c *Controller) FilestoreDelete(ctx types.OwnerContext, path string) error {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return err
	}
	return c.Options.Filestore.Delete(c.Ctx, filePath)
}

// ensureFilestoreAppPath ensures the app's base path exists and returns the full path
func (c *Controller) ensureFilestoreAppPath(appID, path string) (string, error) {
	appPrefix := filestore.GetAppPrefix(c.Options.Config.Controller.FilePrefixGlobal, appID)
	_, err := c.Options.Filestore.CreateFolder(c.Ctx, appPrefix)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(appPrefix, path)
	return fullPath, nil
}

// FilestoreAppList lists files in an app's directory
func (c *Controller) FilestoreAppList(appID, path string) ([]filestore.Item, error) {
	filePath, err := c.ensureFilestoreAppPath(appID, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.List(c.Ctx, filePath)
}

// FilestoreAppGet gets a file from an app's directory
func (c *Controller) FilestoreAppGet(appID, path string) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreAppPath(appID, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.Get(c.Ctx, filePath)
}

// FilestoreAppCreateFolder creates a folder in an app's directory
func (c *Controller) FilestoreAppCreateFolder(appID, path string) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreAppPath(appID, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.CreateFolder(c.Ctx, filePath)
}

// FilestoreAppUploadFile uploads a file to an app's directory
func (c *Controller) FilestoreAppUploadFile(appID, path string, r io.Reader) (filestore.Item, error) {
	filePath, err := c.ensureFilestoreAppPath(appID, path)
	if err != nil {
		return filestore.Item{}, err
	}
	return c.Options.Filestore.WriteFile(c.Ctx, filePath, r)
}

// FilestoreAppDelete deletes a file in an app's directory
func (c *Controller) FilestoreAppDelete(appID, path string) error {
	filePath, err := c.ensureFilestoreAppPath(appID, path)
	if err != nil {
		return err
	}
	return c.Options.Filestore.Delete(c.Ctx, filePath)
}

// IsAppPath checks if a path is within an app's filestore
func IsAppPath(path string) bool {
	return strings.HasPrefix(path, "apps/")
}

// ExtractAppID extracts the app ID from a path that starts with 'apps/'
func ExtractAppID(path string) (string, error) {
	if !IsAppPath(path) {
		return "", fmt.Errorf("not an app path: %s", path)
	}

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid app path format: %s", path)
	}

	return parts[1], nil
}
