package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/model"
)

type AxolotlClient struct {
	client HTTPDoer
	url    string
}

func NewAxolotlClient(_ context.Context, url string) (*AxolotlClient, error) {
	return &AxolotlClient{
		client: http.DefaultClient,
		url:    url,
	}, nil
}

func (d *AxolotlClient) Healthz(ctx context.Context) error {
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
		return fmt.Errorf("axolotl healthz returned status %d", resp.StatusCode)
	}
	return nil
}

type AxolotlVersionResponse struct {
	Version string `json:"version"`
}

func (d *AxolotlClient) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.url+"/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("axolotl version returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var versionResp AxolotlVersionResponse
	if err := json.Unmarshal(body, &versionResp); err != nil {
		return "", err
	}
	return versionResp.Version, nil
}

func parseHelixLoraModelName(m string) (model.Name, string, string, error) {
	splits := strings.Split(m, "?")
	if len(splits) != 3 {
		return "", "", "", fmt.Errorf("invalid model name for pulling helix lora model: %s, must be in the format of <base_model>?<session_id>?<lora_dir>", m)
	}
	return model.NewModel(splits[0]), splits[1], splits[2], nil
}

func buildLocalLoraDir(sessionID string) string {
	return "/tmp/helix/results/" + sessionID
}
