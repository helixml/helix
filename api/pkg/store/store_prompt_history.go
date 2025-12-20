package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// SyncPromptHistory syncs prompt history entries from the frontend
// Uses a simple union operation - new entries are added, existing ones are skipped
func (s *PostgresStore) SyncPromptHistory(ctx context.Context, userID string, req *types.PromptHistorySyncRequest) (*types.PromptHistorySyncResponse, error) {
	synced := 0
	existing := 0

	for _, entry := range req.Entries {
		// Convert timestamp from milliseconds to time.Time
		createdAt := time.UnixMilli(entry.Timestamp)

		// Default interrupt to true if not specified
		interrupt := true
		if entry.Interrupt != nil {
			interrupt = *entry.Interrupt
		}

		// Default pinned to false if not specified
		pinned := false
		if entry.Pinned != nil {
			pinned = *entry.Pinned
		}

		// Default isTemplate to false if not specified
		isTemplate := false
		if entry.IsTemplate != nil {
			isTemplate = *entry.IsTemplate
		}

		dbEntry := &types.PromptHistoryEntry{
			ID:            entry.ID,
			UserID:        userID,
			ProjectID:     req.ProjectID,
			SpecTaskID:    req.SpecTaskID,
			SessionID:     entry.SessionID,
			Content:       entry.Content,
			Status:        entry.Status,
			Interrupt:     interrupt,
			QueuePosition: entry.QueuePosition,
			Pinned:        pinned,
			Tags:          entry.Tags,
			IsTemplate:    isTemplate,
			CreatedAt:     createdAt,
			UpdatedAt:     time.Now(),
		}

		// Use ON CONFLICT DO NOTHING - if entry exists, skip it
		result := s.gdb.WithContext(ctx).
			Where("id = ?", entry.ID).
			FirstOrCreate(dbEntry)

		if result.Error != nil {
			return nil, result.Error
		}

		if result.RowsAffected > 0 {
			synced++
		} else {
			existing++
		}
	}

	// Return all entries for this user+spec_task so client can merge
	var allEntries []types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND spec_task_id = ?", userID, req.SpecTaskID).
		Order("created_at DESC").
		Limit(100). // Reasonable limit
		Find(&allEntries).Error

	if err != nil {
		return nil, err
	}

	return &types.PromptHistorySyncResponse{
		Synced:   synced,
		Existing: existing,
		Entries:  allEntries,
	}, nil
}

// GetPromptHistoryEntry returns a single prompt history entry by ID
func (s *PostgresStore) GetPromptHistoryEntry(ctx context.Context, id string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry
	result := s.gdb.WithContext(ctx).Where("id = ?", id).First(&entry)
	if result.Error != nil {
		if result.Error.Error() == "record not found" {
			return nil, nil
		}
		return nil, result.Error
	}
	return &entry, nil
}

// GetNextPendingPrompt returns the next pending non-interrupt prompt for a session
// Used by the queue processor to send non-interrupt messages after the current conversation completes
func (s *PostgresStore) GetNextPendingPrompt(ctx context.Context, sessionID string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry

	// Find the oldest pending prompt for this session with interrupt=false
	// Order by queue_position (if set) then by created_at
	result := s.gdb.WithContext(ctx).
		Where("session_id = ? AND status = ? AND interrupt = ?", sessionID, "pending", false).
		Order("COALESCE(queue_position, 999999) ASC, created_at ASC").
		First(&entry)

	if result.Error != nil {
		if result.Error.Error() == "record not found" {
			return nil, nil // No pending prompts
		}
		return nil, result.Error
	}

	return &entry, nil
}

// MarkPromptAsSent marks a prompt as sent
func (s *PostgresStore) MarkPromptAsSent(ctx context.Context, promptID string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("status", "sent").
		Error
}

// MarkPromptAsFailed marks a prompt as failed
func (s *PostgresStore) MarkPromptAsFailed(ctx context.Context, promptID string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("status", "failed").
		Error
}

// ListPromptHistory returns prompt history entries for a user
func (s *PostgresStore) ListPromptHistory(ctx context.Context, userID string, req *types.PromptHistoryListRequest) (*types.PromptHistoryListResponse, error) {
	query := s.gdb.WithContext(ctx).
		Where("user_id = ?", userID)

	// Filter by spec task (required - history is per-spec-task)
	if req.SpecTaskID != "" {
		query = query.Where("spec_task_id = ?", req.SpecTaskID)
	}

	// Filter by project if specified (optional additional filter)
	if req.ProjectID != "" {
		query = query.Where("project_id = ?", req.ProjectID)
	}

	// Filter by session if specified
	if req.SessionID != "" {
		query = query.Where("session_id = ?", req.SessionID)
	}

	// Filter by timestamp if specified (for incremental sync)
	if req.Since > 0 {
		sinceTime := time.UnixMilli(req.Since)
		query = query.Where("created_at > ?", sinceTime)
	}

	// Get total count
	var total int64
	if err := query.Model(&types.PromptHistoryEntry{}).Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply limit
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var entries []types.PromptHistoryEntry
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return &types.PromptHistoryListResponse{
		Entries: entries,
		Total:   total,
	}, nil
}

// UpdatePromptPin sets the pinned status of a prompt
func (s *PostgresStore) UpdatePromptPin(ctx context.Context, promptID string, pinned bool) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("pinned", pinned).
		Error
}

// UpdatePromptTags updates the tags of a prompt (JSON array string)
func (s *PostgresStore) UpdatePromptTags(ctx context.Context, promptID string, tags string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("tags", tags).
		Error
}

// UpdatePromptTemplate sets whether a prompt is a template
func (s *PostgresStore) UpdatePromptTemplate(ctx context.Context, promptID string, isTemplate bool) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("is_template", isTemplate).
		Error
}

// ListPinnedPrompts returns all pinned prompts for a user in a spec task
func (s *PostgresStore) ListPinnedPrompts(ctx context.Context, userID, specTaskID string) ([]*types.PromptHistoryEntry, error) {
	var entries []*types.PromptHistoryEntry
	query := s.gdb.WithContext(ctx).
		Where("user_id = ? AND pinned = ?", userID, true)

	if specTaskID != "" {
		query = query.Where("spec_task_id = ?", specTaskID)
	}

	err := query.
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// ListPromptTemplates returns all templates for a user (across all projects)
func (s *PostgresStore) ListPromptTemplates(ctx context.Context, userID string) ([]*types.PromptHistoryEntry, error) {
	var entries []*types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND is_template = ?", userID, true).
		Order("usage_count DESC, created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// IncrementPromptUsage increments usage count and updates last_used_at
func (s *PostgresStore) IncrementPromptUsage(ctx context.Context, promptID string) error {
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Updates(map[string]interface{}{
			"usage_count":  s.gdb.Raw("usage_count + 1"),
			"last_used_at": now,
		}).
		Error
}

// SearchPrompts searches prompts by content using ILIKE (case-insensitive)
func (s *PostgresStore) SearchPrompts(ctx context.Context, userID, query string, limit int) ([]*types.PromptHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var entries []*types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND content ILIKE ?", userID, "%"+query+"%").
		Order("pinned DESC, usage_count DESC, created_at DESC").
		Limit(limit).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// UnifiedSearch searches across projects, tasks, sessions, and prompts
func (s *PostgresStore) UnifiedSearch(ctx context.Context, userID string, req *types.UnifiedSearchRequest) (*types.UnifiedSearchResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}

	results := make([]types.UnifiedSearchResult, 0)
	searchQuery := "%" + req.Query + "%"

	// Determine which types to search
	searchTypes := req.Types
	if len(searchTypes) == 0 {
		searchTypes = []string{"projects", "tasks", "sessions", "prompts", "knowledge", "repositories", "agents"}
	}

	for _, searchType := range searchTypes {
		switch searchType {
		case "projects":
			// Search projects
			var projects []types.Project
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if req.OrgID != "" {
				query = query.Where("owner = ? AND owner_type = ?", req.OrgID, types.OwnerTypeOrg)
			}

			if err := query.Find(&projects).Error; err == nil {
				for _, p := range projects {
					results = append(results, types.UnifiedSearchResult{
						Type:        "project",
						ID:          p.ID,
						Title:       p.Name,
						Description: truncateText(p.Description, 150),
						URL:         "/projects/" + p.ID,
						Icon:        "folder",
						Metadata: map[string]string{
							"status": p.Status,
						},
						CreatedAt: p.CreatedAt.Format(time.RFC3339),
						UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "tasks":
			// Search spec tasks
			var tasks []types.SpecTask
			query := s.gdb.WithContext(ctx).
				Where("owner_id = ? AND (name ILIKE ? OR original_prompt ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if err := query.Find(&tasks).Error; err == nil {
				// Collect project IDs to fetch names
				projectIDs := make([]string, 0)
				for _, t := range tasks {
					if t.ProjectID != "" {
						projectIDs = append(projectIDs, t.ProjectID)
					}
				}

				// Fetch project names in bulk
				projectNames := make(map[string]string)
				if len(projectIDs) > 0 {
					var projects []types.Project
					if err := s.gdb.WithContext(ctx).
						Select("id", "name").
						Where("id IN ?", projectIDs).
						Find(&projects).Error; err == nil {
						for _, p := range projects {
							projectNames[p.ID] = p.Name
						}
					}
				}

				for _, t := range tasks {
					meta := map[string]string{
						"status": string(t.Status),
					}
					if t.ProjectID != "" {
						meta["projectId"] = t.ProjectID
						if name, ok := projectNames[t.ProjectID]; ok {
							meta["projectName"] = name
						}
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "task",
						ID:          t.ID,
						Title:       t.Name,
						Description: truncateText(t.OriginalPrompt, 150),
						URL:         "/tasks/" + t.ID,
						Icon:        "task",
						Metadata:    meta,
						CreatedAt:   t.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "sessions":
			// Search sessions by name
			var sessions []types.Session
			query := s.gdb.WithContext(ctx).
				Preload("Interactions", func(db *gorm.DB) *gorm.DB {
					return db.Limit(1).Order("created ASC")
				}).
				Where("owner = ? AND name ILIKE ?", userID, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&sessions).Error; err == nil {
				// Collect task IDs to fetch names
				taskIDs := make([]string, 0)
				for _, sess := range sessions {
					if sess.Metadata.SpecTaskID != "" {
						taskIDs = append(taskIDs, sess.Metadata.SpecTaskID)
					}
				}

				// Fetch task names in bulk
				taskNames := make(map[string]string)
				if len(taskIDs) > 0 {
					var tasks []types.SpecTask
					if err := s.gdb.WithContext(ctx).
						Select("id", "name").
						Where("id IN ?", taskIDs).
						Find(&tasks).Error; err == nil {
						for _, t := range tasks {
							taskNames[t.ID] = t.Name
						}
					}
				}

				for _, sess := range sessions {
					var desc string
					if len(sess.Interactions) > 0 {
						desc = truncateText(sess.Interactions[0].PromptMessage, 150)
					}

					meta := map[string]string{
						"mode": string(sess.Mode),
					}
					if sess.ParentApp != "" {
						meta["appId"] = sess.ParentApp
					}
					if sess.Metadata.SpecTaskID != "" {
						meta["taskId"] = sess.Metadata.SpecTaskID
						if name, ok := taskNames[sess.Metadata.SpecTaskID]; ok {
							meta["taskName"] = name
						}
					}
					if sess.Metadata.ProjectID != "" {
						meta["projectId"] = sess.Metadata.ProjectID
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "session",
						ID:          sess.ID,
						Title:       sess.Name,
						Description: desc,
						URL:         "/session/" + sess.ID,
						Icon:        "chat",
						Metadata:    meta,
						CreatedAt:   sess.Created.Format(time.RFC3339),
						UpdatedAt:   sess.Updated.Format(time.RFC3339),
					})
				}
			}

		case "prompts":
			// Search prompt history
			var prompts []*types.PromptHistoryEntry
			query := s.gdb.WithContext(ctx).
				Where("user_id = ? AND content ILIKE ?", userID, searchQuery).
				Limit(req.Limit).
				Order("pinned DESC, created_at DESC")

			if err := query.Find(&prompts).Error; err == nil {
				for _, p := range prompts {
					meta := map[string]string{
						"status":    p.Status,
						"projectId": p.ProjectID,
						"taskId":    p.SpecTaskID,
					}
					if p.Pinned {
						meta["pinned"] = "true"
					}
					if p.IsTemplate {
						meta["template"] = "true"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "prompt",
						ID:          p.ID,
						Title:       truncateText(p.Content, 80),
						Description: truncateText(p.Content, 200),
						URL:         "/tasks/" + p.SpecTaskID,
						Icon:        "prompt",
						Metadata:    meta,
						CreatedAt:   p.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "knowledge":
			// Search knowledge sources (RAG)
			var knowledgeList []*types.Knowledge
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&knowledgeList).Error; err == nil {
				for _, k := range knowledgeList {
					sourceType := getKnowledgeSourceType(k)
					meta := map[string]string{
						"state":      string(k.State),
						"sourceType": sourceType,
					}
					if k.AppID != "" {
						meta["appId"] = k.AppID
					}

					// Navigate to the app that contains this knowledge
					url := "/knowledge/" + k.ID
					if k.AppID != "" {
						url = "/app/" + k.AppID + "/knowledge"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "knowledge",
						ID:          k.ID,
						Title:       k.Name,
						Description: truncateText(k.Description, 150),
						URL:         url,
						Icon:        "knowledge",
						Metadata:    meta,
						CreatedAt:   k.Created.Format(time.RFC3339),
						UpdatedAt:   k.Updated.Format(time.RFC3339),
					})
				}
			}

		case "repositories":
			// Search git repositories
			var repos []types.GitRepository
			query := s.gdb.WithContext(ctx).
				Where("owner_id = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if err := query.Find(&repos).Error; err == nil {
				for _, r := range repos {
					meta := map[string]string{}
					if r.ExternalURL != "" {
						meta["externalUrl"] = r.ExternalURL
					}
					if r.KoditIndexing {
						meta["koditIndexing"] = "true"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "repository",
						ID:          r.ID,
						Title:       r.Name,
						Description: truncateText(r.Description, 150),
						URL:         "/repositories/" + r.ID,
						Icon:        "repository",
						Metadata:    meta,
						CreatedAt:   r.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "agents":
			// Search apps/agents - name and description are in the JSONB config field
			var apps []types.App
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (config->'helix'->>'name' ILIKE ? OR config->'helix'->>'description' ILIKE ?)",
					userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&apps).Error; err == nil {
				for _, a := range apps {
					name := a.Config.Helix.Name
					if name == "" {
						name = "Untitled Agent"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "agent",
						ID:          a.ID,
						Title:       name,
						Description: truncateText(a.Config.Helix.Description, 150),
						URL:         "/app/" + a.ID,
						Icon:        "agent",
						CreatedAt:   a.Created.Format(time.RFC3339),
						UpdatedAt:   a.Updated.Format(time.RFC3339),
					})
				}
			}
		}
	}

	return &types.UnifiedSearchResponse{
		Results: results,
		Total:   len(results),
		Query:   req.Query,
	}, nil
}

// truncateText truncates text to maxLen, adding ellipsis if needed
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// getKnowledgeSourceType returns a human-readable source type for knowledge
func getKnowledgeSourceType(k *types.Knowledge) string {
	if k.Source.Web != nil {
		return "web"
	}
	if k.Source.Filestore != nil {
		return "files"
	}
	if k.Source.S3 != nil {
		return "s3"
	}
	if k.Source.GCS != nil {
		return "gcs"
	}
	if k.Source.SharePoint != nil {
		return "sharepoint"
	}
	if k.Source.Text != nil {
		return "text"
	}
	return "unknown"
}
