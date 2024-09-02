package client

import (
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

type KnowledgeFilter struct {
	AppID string
}

func (c *HelixClient) ListKnowledge(f *KnowledgeFilter) ([]*types.Knowledge, error) {
	path := "/knowledge"
	if f.AppID != "" {
		path += "?app_id=" + f.AppID
	}

	var knowledge []*types.Knowledge

	err := c.makeRequest(http.MethodGet, path, nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) GetKnowledge(id string) (*types.Knowledge, error) {
	path := "/knowledge/" + id

	var knowledge *types.Knowledge
	err := c.makeRequest(http.MethodGet, path, nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) DeleteKnowledge(id string) error {
	err := c.makeRequest(http.MethodDelete, "/knowledge/"+id, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge, %w", err)
	}
	return nil
}

func (c *HelixClient) RefreshKnowledge(id string) error {
	err := c.makeRequest(http.MethodPost, "/knowledge/"+id+"/refresh", nil, nil)
	if err != nil {
		return fmt.Errorf("failed to refresh knowledge, %w", err)
	}

	return nil
}

type KnowledgeVersionsFilter struct {
	KnowledgeID string
}

func (c *HelixClient) ListKnowledgeVersions(f *KnowledgeVersionsFilter) ([]*types.KnowledgeVersion, error) {
	var knowledge []*types.KnowledgeVersion
	err := c.makeRequest(http.MethodGet, fmt.Sprintf("/knowledge/%s/versions", f.KnowledgeID), nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}
