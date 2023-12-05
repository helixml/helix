package controller

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"path/filepath"
	"text/template"

	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type userPathTemplateData struct {
	Owner     string
	OwnerType types.OwnerType
}

//go:embed filestore_folders.json
var jsonFile embed.FS

func GetFolders() ([]filestore.FilestoreFolder, error) {
	file, err := jsonFile.Open("filestore_folders.json")
	if err != nil {
		return []filestore.FilestoreFolder{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return []filestore.FilestoreFolder{}, err
	}

	var folders []filestore.FilestoreFolder
	if err := json.Unmarshal(content, &folders); err != nil {
		return []filestore.FilestoreFolder{}, err
	}

	return folders, nil
}

func GetSessionFolder(sessionID string) string {
	return filepath.Join("sessions", sessionID)
}

func GetSessionInputsFolder(sessionID string) string {
	return filepath.Join(GetSessionFolder(sessionID), "inputs")
}

func GetSessionResultsFolder(sessionID string) string {
	return filepath.Join(GetSessionFolder(sessionID), "results")
}

// apply the user path template so we know what the users prefix actually is
// then return that path with the requested path appended
func (c *Controller) getFilestoreUserPrefix(ctx types.RequestContext) (string, error) {
	tmpl, err := template.New("user_path").Parse(c.Options.FilePrefixUser)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, userPathTemplateData{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (c *Controller) GetFilestoreUserPath(ctx types.RequestContext, path string) (string, error) {
	userPrefix, err := c.getFilestoreUserPrefix(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(c.Options.FilePrefixGlobal, userPrefix, path), nil
}

// given a path - we might have never seen this filestore yet
// so, we must ensure that the users base path is created and then create each
// special folder as listed above
func (c *Controller) ensureFilestoreUserPath(ctx types.RequestContext, path string) (string, error) {
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

func (c *Controller) FilestoreConfig(ctx types.RequestContext) (filestore.FilestoreConfig, error) {
	userPrefix, err := c.getFilestoreUserPrefix(ctx)
	if err != nil {
		return filestore.FilestoreConfig{}, err
	}
	folders, err := GetFolders()
	if err != nil {
		return filestore.FilestoreConfig{}, err
	}
	return filestore.FilestoreConfig{
		UserPrefix: filepath.Join(c.Options.FilePrefixGlobal, userPrefix),
		Folders:    folders,
	}, nil
}

func (c *Controller) FilestoreList(ctx types.RequestContext, path string) ([]filestore.FileStoreItem, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.List(c.Ctx, filePath)
}

func (c *Controller) FilestoreGet(ctx types.RequestContext, path string) (filestore.FileStoreItem, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	return c.Options.Filestore.Get(c.Ctx, filePath)
}

func (c *Controller) FilestoreCreateFolder(ctx types.RequestContext, path string) (filestore.FileStoreItem, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	return c.Options.Filestore.CreateFolder(c.Ctx, filePath)
}

func (c *Controller) FilestoreDownloadFile(ctx types.RequestContext, path string) (io.Reader, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.DownloadFile(c.Ctx, filePath)
}

func (c *Controller) FilestoreDownloadFolder(ctx types.RequestContext, path string) (io.Reader, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.Options.Filestore.DownloadFolder(c.Ctx, filePath)
}

func (c *Controller) FilestoreUploadFile(ctx types.RequestContext, path string, r io.Reader) (filestore.FileStoreItem, error) {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	return c.Options.Filestore.UploadFile(c.Ctx, filePath, r)
}

func (c *Controller) FilestoreRename(ctx types.RequestContext, path string, newPath string) (filestore.FileStoreItem, error) {
	fullPath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	fullNewPath, err := c.ensureFilestoreUserPath(ctx, newPath)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	return c.Options.Filestore.Rename(c.Ctx, fullPath, fullNewPath)
}

func (c *Controller) FilestoreDelete(ctx types.RequestContext, path string) error {
	filePath, err := c.ensureFilestoreUserPath(ctx, path)
	if err != nil {
		return err
	}
	return c.Options.Filestore.Delete(c.Ctx, filePath)
}
