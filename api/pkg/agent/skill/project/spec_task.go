package project

import (
	"fmt"
	"time"
)

// SpecTaskSummary is a summary of a spec task
type SpecTaskSummary struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	BranchName     string     `json:"branch_name,omitempty"`
	PullRequestID  string     `json:"pull_request_id,omitempty"`
	PullRequestURL string     `json:"pull_request_url,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

func (s *SpecTaskSummary) ToString() string {
	result := fmt.Sprintf("ID: %s\nTask: %s\nStatus: %s", s.ID, s.Name, s.Status)
	if s.Description != "" {
		result += fmt.Sprintf("\nDescription: %s", s.Description)
	}
	if s.Priority != "" {
		result += fmt.Sprintf("\nPriority: %s", s.Priority)
	}
	if s.BranchName != "" {
		result += fmt.Sprintf("\nBranchName: %s", s.BranchName)
	}
	if s.PullRequestID != "" {
		result += fmt.Sprintf("\nPullRequestID: %s", s.PullRequestID)
	}
	if s.PullRequestURL != "" {
		result += fmt.Sprintf("\nPullRequestURL: %s", s.PullRequestURL)
	}
	if s.StartedAt != nil {
		result += fmt.Sprintf("\nStartedAt: %s", s.StartedAt.Format(time.RFC3339))
	}
	if s.CompletedAt != nil {
		result += fmt.Sprintf("\nCompletedAt: %s", s.CompletedAt.Format(time.RFC3339))
	}
	return result
}

type ListSpecTasksResult struct {
	Tasks []SpecTaskSummary `json:"tasks"`
	Total int               `json:"total"`
}

func (r *ListSpecTasksResult) ToString() string {
	var result string
	result = fmt.Sprintf("Total Tasks: %d\n\n", r.Total)
	for i, task := range r.Tasks {
		result += fmt.Sprintf("--- Task %d ---\n%s\n\n", i+1, task.ToString())
	}
	return result
}
