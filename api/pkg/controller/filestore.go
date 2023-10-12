package controller

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/bacalhau-project/lilysaas/api/pkg/filestore"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/davecgh/go-spew/spew"
)

const USERS_PATH = "users"

// prefix the path for the currently logged in user
// so we can mutiplex users into one folder of our underlying storage
func (c *Controller) getFilestorePath(ctx types.RequestContext, path string) string {
	return filepath.Join(c.Options.FilestorePrefix, USERS_PATH, ctx.Owner, path)
}

func (c *Controller) ensureFilestoreUserPath(ctx types.RequestContext) error {
	_, err := c.Options.Filestore.CreateFolder(c.Ctx, c.getFilestorePath(ctx, ""))
	return err
}

func (c *Controller) FilestoreList(ctx types.RequestContext, path string) ([]filestore.FileStoreItem, error) {
	err := c.ensureFilestoreUserPath(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Options.Filestore.List(c.Ctx, c.getFilestorePath(ctx, path))
	fmt.Printf("res --------------------------------------\n")
	spew.Dump(res)
	spew.Dump(err)
	return res, err
}

func (c *Controller) FilestoreGet(ctx types.RequestContext, path string) (filestore.FileStoreItem, error) {
	err := c.ensureFilestoreUserPath(ctx)
	if err != nil {
		return filestore.FileStoreItem{}, err
	}
	return c.Options.Filestore.Get(c.Ctx, c.getFilestorePath(ctx, path))
}

func (c *Controller) FilestoreCreateFolder(ctx types.RequestContext, path string) (filestore.FileStoreItem, error) {
	return c.Options.Filestore.CreateFolder(c.Ctx, c.getFilestorePath(ctx, path))
}

func (c *Controller) FilestoreUpload(ctx types.RequestContext, path string, r io.Reader) (filestore.FileStoreItem, error) {
	return c.Options.Filestore.Upload(c.Ctx, c.getFilestorePath(ctx, path), r)
}

func (c *Controller) FilestoreRename(ctx types.RequestContext, path string, newPath string) (filestore.FileStoreItem, error) {
	return c.Options.Filestore.Rename(c.Ctx, c.getFilestorePath(ctx, path), c.getFilestorePath(ctx, newPath))
}

func (c *Controller) FilestoreDelete(ctx types.RequestContext, path string) error {
	return c.Options.Filestore.Delete(c.Ctx, c.getFilestorePath(ctx, path))
}
