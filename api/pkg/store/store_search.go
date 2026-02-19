package store

import (
	"context"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// ResourceSearch - searches across projects, spec tasks, sessions, prompts, knowledge, repositories, apps (agents)
// Launches the search in parallel for each resource type and returns the results
func (s *PostgresStore) ResourceSearch(ctx context.Context, req *types.ResourceSearchRequest) (*types.ResourceSearchResponse, error) {
	if req.Limit == 0 {
		req.Limit = 10
	}

	// Default to all searchable types if none specified
	searchTypes := req.Types
	if len(searchTypes) == 0 {
		searchTypes = []types.Resource{
			types.ResourceProject,
			types.ResourceSpecTask,
			types.ResourceSession,
			types.ResourcePrompt,
			types.ResourceKnowledge,
			types.ResourceGitRepository,
			types.ResourceApplication,
		}
	}

	var (
		results   []types.ResourceSearchResult
		resultsMu sync.Mutex
		wg        sync.WaitGroup
	)

	query := strings.ToLower(req.Query)

	for _, resourceType := range searchTypes {
		wg.Add(1)
		go func(rt types.Resource) {
			defer wg.Done()

			var items []types.ResourceSearchResult
			var err error

			switch rt {
			case types.ResourceProject:
				items, err = s.searchProjects(ctx, query, req)
			case types.ResourceSpecTask:
				items, err = s.searchSpecTasks(ctx, query, req)
			case types.ResourceSession:
				items, err = s.searchSessions(ctx, query, req)
			case types.ResourcePrompt:
				items, err = s.searchPrompts(ctx, query, req)
			case types.ResourceKnowledge:
				items, err = s.searchKnowledge(ctx, query, req)
			case types.ResourceGitRepository:
				items, err = s.searchGitRepositories(ctx, query, req)
			case types.ResourceApplication:
				items, err = s.searchApps(ctx, query, req)
			}

			if err != nil {
				// Log error but continue with other searches
				return
			}

			resultsMu.Lock()
			results = append(results, items...)
			resultsMu.Unlock()
		}(resourceType)
	}

	wg.Wait()

	return &types.ResourceSearchResponse{
		Results: results,
		Total:   len(results),
	}, nil
}

func (s *PostgresStore) searchProjects(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var projects []*types.Project

	q := s.gdb.WithContext(ctx).Model(&types.Project{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("user_id = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	// Name: prefix match, Description: contains match
	q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", query+"%", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&projects).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(projects))
	for _, p := range projects {
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceProject,
			ResourceID:          p.ID,
			ResourceName:        p.Name,
			ResourceDescription: p.Description,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchSpecTasks(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var tasks []*types.SpecTask

	q := s.gdb.WithContext(ctx).Model(&types.SpecTask{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("user_id = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	q = q.Where("archived = false").
		Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(original_prompt) LIKE ?",
			query+"%", "%"+query+"%", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&tasks).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(tasks))
	for _, t := range tasks {
		contents := t.OriginalPrompt
		if contents == "" && t.Description != "" {
			contents = t.Description
		}
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceSpecTask,
			ResourceID:          t.ID,
			ResourceName:        t.Name,
			ResourceDescription: t.Description,
			Contents:            contents,
			ParentID:            t.ProjectID,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchSessions(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var sessions []*types.Session

	q := s.gdb.WithContext(ctx).Model(&types.Session{}).
		Preload("Interactions", func(db *gorm.DB) *gorm.DB {
			return db.Order("created ASC").Limit(1)
		}).
		Where("LOWER(name) LIKE ?", query+"%")

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("owner = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	err := q.Limit(req.Limit).Find(&sessions).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(sessions))
	for _, sess := range sessions {
		var contents string
		if len(sess.Interactions) > 0 {
			contents = sess.Interactions[0].PromptMessage
		}
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceSession,
			ResourceID:          sess.ID,
			ResourceName:        sess.Name,
			ResourceDescription: "",
			Contents:            contents,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchPrompts(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var prompts []*types.PromptHistoryEntry
	q := s.gdb.WithContext(ctx).Model(&types.PromptHistoryEntry{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("user_id = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	q = q.Where("LOWER(content) LIKE ?", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&prompts).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(prompts))
	for _, p := range prompts {
		desc := p.Content
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourcePrompt,
			ResourceID:          p.ID,
			ResourceName:        desc,
			ResourceDescription: "",
			Contents:            p.Content,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchKnowledge(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var knowledge []*types.Knowledge

	q := s.gdb.WithContext(ctx).Model(&types.Knowledge{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("owner = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	// Name: prefix match, Description: contains match
	q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", query+"%", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&knowledge).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(knowledge))
	for _, k := range knowledge {
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceKnowledge,
			ResourceID:          k.ID,
			ResourceName:        k.Name,
			ResourceDescription: k.Description,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchGitRepositories(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var repos []*types.GitRepository

	q := s.gdb.WithContext(ctx).Model(&types.GitRepository{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("owner_id = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	// Name: prefix match, Description: contains match
	q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", query+"%", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&repos).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(repos))
	for _, r := range repos {
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceGitRepository,
			ResourceID:          r.ID,
			ResourceName:        r.Name,
			ResourceDescription: r.Description,
		})
	}
	return results, nil
}

func (s *PostgresStore) searchApps(ctx context.Context, query string, req *types.ResourceSearchRequest) ([]types.ResourceSearchResult, error) {
	var apps []*types.App

	q := s.gdb.WithContext(ctx).Model(&types.App{})

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		q = q.Where("owner = ? AND (organization_id = '' OR organization_id IS NULL)", req.UserID)
	}

	// Name: prefix match, Description: contains match
	// App.Config is AppConfig with nested Helix field: config->'helix'->>'name'
	q = q.Where("LOWER(config->'helix'->>'name') LIKE ? OR LOWER(config->'helix'->>'description') LIKE ?", query+"%", "%"+query+"%")

	err := q.Limit(req.Limit).Find(&apps).Error
	if err != nil {
		return nil, err
	}

	results := make([]types.ResourceSearchResult, 0, len(apps))
	for _, a := range apps {
		results = append(results, types.ResourceSearchResult{
			ResourceType:        types.ResourceApplication,
			ResourceID:          a.ID,
			ResourceName:        a.Config.Helix.Name,
			ResourceDescription: a.Config.Helix.Description,
		})
	}
	return results, nil
}
