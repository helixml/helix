package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helixml/helix/api/pkg/types"
)

type KnowledgeFilter struct {
	AppID string
}

func (c *HelixClient) ListKnowledge(ctx context.Context, f *KnowledgeFilter) ([]*types.Knowledge, error) {
	path := "/knowledge"
	if f.AppID != "" {
		path += "?app_id=" + f.AppID
	}

	var knowledge []*types.Knowledge

	err := c.makeRequest(ctx, http.MethodGet, path, nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	path := "/knowledge/" + id

	var knowledge *types.Knowledge
	err := c.makeRequest(ctx, http.MethodGet, path, nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func (c *HelixClient) DeleteKnowledge(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodDelete, "/knowledge/"+id, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge, %w", err)
	}
	return nil
}

func (c *HelixClient) RefreshKnowledge(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodPost, "/knowledge/"+id+"/refresh", nil, nil)
	if err != nil {
		return fmt.Errorf("failed to refresh knowledge, %w", err)
	}

	return nil
}

func (c *HelixClient) CompleteKnowledgePreparation(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodPost, "/knowledge/"+id+"/complete", nil, nil)
	if err != nil {
		return fmt.Errorf("failed to complete knowledge preparation, %w", err)
	}

	return nil
}

type KnowledgeVersionsFilter struct {
	KnowledgeID string
}

func (c *HelixClient) ListKnowledgeVersions(ctx context.Context, f *KnowledgeVersionsFilter) ([]*types.KnowledgeVersion, error) {
	var knowledge []*types.KnowledgeVersion
	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/knowledge/%s/versions", f.KnowledgeID), nil, &knowledge)
	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

type KnowledgeSearchQuery struct {
	AppID       string
	KnowledgeID string
	Prompt      string
}

func (c *HelixClient) SearchKnowledge(ctx context.Context, f *KnowledgeSearchQuery) ([]*types.KnowledgeSearchResult, error) {
	var result []*types.KnowledgeSearchResult

	params := url.Values{}
	params.Add("app_id", f.AppID)
	params.Add("prompt", f.Prompt)
	if f.KnowledgeID != "" {
		params.Add("knowledge_id", f.KnowledgeID)
	}

	path := "/search?" + params.Encode()
	err := c.makeRequest(ctx, http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
