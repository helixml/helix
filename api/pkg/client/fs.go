package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"

	"github.com/helixml/helix/api/pkg/filestore"
)

func (c *HelixClient) FilestoreList(ctx context.Context, path string) ([]filestore.Item, error) {
	var resp []filestore.Item

	url := url.URL{
		Path: "/filestore/list",
	}

	query := url.Query()
	query.Add("path", path)
	url.RawQuery = query.Encode()

	err := c.makeRequest(ctx, http.MethodGet, url.String(), nil, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *HelixClient) FilestoreDelete(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	err := c.makeRequest(ctx, http.MethodDelete, "/filestore/delete?path="+path, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *HelixClient) FilestoreUpload(ctx context.Context, path string, file io.Reader) error {
	path = strings.ReplaceAll(path, `\`, "/")
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("path must include a filename")
	}
	filename := pathpkg.Base(path)
	directory := pathpkg.Dir(path)
	if filename == "." || filename == "/" {
		return fmt.Errorf("path must include a filename")
	}
	if directory == "." {
		directory = ""
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	url := url.URL{
		Path: "/filestore/upload",
	}

	query := url.Query()
	query.Add("path", directory)
	url.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+url.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload file: %s", resp.Status)
	}

	return nil
}

// FilestoreGet returns the metadata for one filestore path. The API responds
// with an item whose Path is the canonical (owner-prefixed) path that
// FilestoreRead accepts.
func (c *HelixClient) FilestoreGet(ctx context.Context, path string) (*filestore.Item, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	u := url.URL{Path: "/filestore/get"}
	q := u.Query()
	q.Add("path", path)
	u.RawQuery = q.Encode()

	var item filestore.Item
	if err := c.makeRequest(ctx, http.MethodGet, u.String(), nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// FilestoreRead downloads a file's bytes by its canonical filestore path (as
// returned in filestore.Item.Path, e.g. "dev/users/<id>/engagements/x/y.json").
// A user-relative path is resolved with FilestoreGet first.
func (c *HelixClient) FilestoreRead(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/filestore/viewer/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		bts, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("status code %d (%s)", resp.StatusCode, string(bts))
	}
	return io.ReadAll(resp.Body)
}
