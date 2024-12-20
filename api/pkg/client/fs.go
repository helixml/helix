package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/filestore"
)

func (c *HelixClient) FilestoreList(ctx context.Context, path string) ([]filestore.FileStoreItem, error) {
	var resp []filestore.FileStoreItem

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
	if path == "" {
		return fmt.Errorf("path is required")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filepath.Base(path))
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

	// Remove the filename from the path as it would create a directory named as a filename
	path = filepath.Dir(path)

	url := url.URL{
		Path: "/filestore/upload",
	}

	query := url.Query()
	query.Add("path", path)
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
