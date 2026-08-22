package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

type ArtifactContent struct {
	Filename string
	Reader   io.Reader
}

type ArtifactUploadRequest struct {
	Name             *string
	Description      *string
	Entrypoint       *string
	Visibility       *types.ArtifactVisibility
	WithSubdomain    *bool
	SourceSessionID  string
	SourceSpecTaskID string
	Content          *ArtifactContent
}

func (c *HelixClient) ListArtifacts(ctx context.Context, projectID string) ([]*types.Artifact, error) {
	var response types.ArtifactsListResponse
	if err := c.makeRequest(ctx, http.MethodGet, "/projects/"+projectID+"/artifacts", nil, &response); err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (c *HelixClient) GetArtifact(ctx context.Context, artifactID string) (*types.Artifact, error) {
	var artifact types.Artifact
	if err := c.makeRequest(ctx, http.MethodGet, "/artifacts/"+artifactID, nil, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (c *HelixClient) CreateArtifact(ctx context.Context, projectID string, input *ArtifactUploadRequest) (*types.Artifact, error) {
	return c.uploadArtifact(ctx, http.MethodPost, "/projects/"+projectID+"/artifacts", input)
}

func (c *HelixClient) UpdateArtifact(ctx context.Context, artifactID string, input *ArtifactUploadRequest) (*types.Artifact, error) {
	return c.uploadArtifact(ctx, http.MethodPut, "/artifacts/"+artifactID, input)
}

func (c *HelixClient) DeleteArtifact(ctx context.Context, artifactID string) error {
	return c.makeRequest(ctx, http.MethodDelete, "/artifacts/"+artifactID, nil, nil)
}

func (c *HelixClient) uploadArtifact(ctx context.Context, method, requestPath string, input *ArtifactUploadRequest) (*types.Artifact, error) {
	if input == nil {
		return nil, fmt.Errorf("artifact input is required")
	}
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)

	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, c.url+requestPath, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		return nil, fmt.Errorf("create artifact request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	go func() {
		err := writeArtifactMultipart(multipartWriter, input)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = pipeWriter.CloseWithError(err)
	}()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		message, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return nil, fmt.Errorf("artifact API returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("artifact API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	var artifact types.Artifact
	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		return nil, fmt.Errorf("decode artifact response: %w", err)
	}
	return &artifact, nil
}

func writeArtifactMultipart(writer *multipart.Writer, input *ArtifactUploadRequest) error {
	fields := []struct {
		name  string
		value *string
	}{
		{name: "name", value: input.Name},
		{name: "description", value: input.Description},
		{name: "entrypoint", value: input.Entrypoint},
	}
	for _, field := range fields {
		if field.value != nil {
			if err := writer.WriteField(field.name, *field.value); err != nil {
				return fmt.Errorf("write artifact field %s: %w", field.name, err)
			}
		}
	}
	if input.Visibility != nil {
		if err := writer.WriteField("visibility", string(*input.Visibility)); err != nil {
			return fmt.Errorf("write artifact visibility: %w", err)
		}
	}
	if input.WithSubdomain != nil {
		if err := writer.WriteField("with_subdomain", strconv.FormatBool(*input.WithSubdomain)); err != nil {
			return fmt.Errorf("write artifact subdomain setting: %w", err)
		}
	}
	if input.SourceSessionID != "" {
		if err := writer.WriteField("source_session_id", input.SourceSessionID); err != nil {
			return fmt.Errorf("write source session: %w", err)
		}
	}
	if input.SourceSpecTaskID != "" {
		if err := writer.WriteField("source_spec_task_id", input.SourceSpecTaskID); err != nil {
			return fmt.Errorf("write source spec task: %w", err)
		}
	}
	if input.Content == nil {
		return nil
	}
	if input.Content.Filename == "" || input.Content.Reader == nil {
		return fmt.Errorf("artifact content filename and reader are required")
	}
	part, err := writer.CreateFormFile("artifact", input.Content.Filename)
	if err != nil {
		return fmt.Errorf("create artifact multipart file: %w", err)
	}
	if _, err := io.Copy(part, input.Content.Reader); err != nil {
		return fmt.Errorf("write artifact content: %w", err)
	}
	return nil
}
