package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

type KnowledgeFilter struct {
}

func (c *HelixClient) ListKnowledge(f *KnowledgeFilter) ([]*types.Knowledge, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/knowledge", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var knowledge []*types.Knowledge
	err = json.Unmarshal(bts, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) GetKnowledge(id string) (*types.Knowledge, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/knowledge/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var knowledge *types.Knowledge
	err = json.NewDecoder(resp.Body).Decode(&knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) DeleteKnowledge(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.url+"/knowledge/"+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete app, status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HelixClient) RefreshKnowledge(id string) error {
	req, err := http.NewRequest(http.MethodPost, c.url+"/knowledge/"+id+"/refresh", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete app, status code: %d", resp.StatusCode)
	}

	return nil
}
