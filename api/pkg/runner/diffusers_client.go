package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type DiffusersClient struct {
	client HTTPDoer
	url    string
}

func NewDiffusersClient(_ context.Context, url string) (*DiffusersClient, error) {
	return &DiffusersClient{
		client: http.DefaultClient,
		url:    url,
	}, nil
}

func (d *DiffusersClient) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", d.url+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers healthz returned status %d", resp.StatusCode)
	}
	return nil
}

type DiffusersVersionResponse struct {
	Version string `json:"version"`
}

func (d *DiffusersClient) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.url+"/version", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("diffusers version returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}
	var versionResp DiffusersVersionResponse
	if err := json.Unmarshal(body, &versionResp); err != nil {
		return "", fmt.Errorf("error unmarshalling response: %w", err)
	}
	return versionResp.Version, nil
}

type DiffusersPullRequest struct {
	Model string `json:"model"`
}

func (d *DiffusersClient) Pull(ctx context.Context, modelName string) error {
	pullReq := DiffusersPullRequest{
		Model: modelName,
	}
	body, err := json.Marshal(pullReq)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/pull", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers pull returned status %d", resp.StatusCode)
	}
	return nil
}

type DiffusersWarmRequest struct {
	Model string `json:"model"`
}

func (d *DiffusersClient) Warm(ctx context.Context, modelName string) error {
	warmReq := DiffusersWarmRequest{
		Model: modelName,
	}
	body, err := json.Marshal(warmReq)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/warm", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers warm returned status %d: %s", resp.StatusCode, responseBody)
	}
	return nil
}

type GenerateStreamingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
	N      int    `json:"n"`
}

func (d *DiffusersClient) GenerateStreaming(ctx context.Context, prompt string, callback func(types.HelixImageGenerationUpdate) error) error {
	// Create request body
	body, err := json.Marshal(GenerateStreamingRequest{
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/v1/images/generations/stream", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers generate streaming returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines or non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		log.Debug().Str("line", line).Msg("Received line")
		// Extract the JSON payload after "data: "
		jsonData := strings.TrimPrefix(line, "data: ")

		// Skip keep-alive messages
		if jsonData == "" || jsonData == ":keep-alive" {
			continue
		}

		// Parse the JSON update
		var update types.HelixImageGenerationUpdate
		if err := json.Unmarshal([]byte(jsonData), &update); err != nil {
			return fmt.Errorf("error parsing response: %w", err)
		}

		// Call the callback with the update
		if err := callback(update); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	return nil
}
