package project

import (
	"fmt"
	"time"
)

// SpecTaskSummary is a summary of a spec task
type SpecTaskSummary struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Description      string              `json:"description"`
	Status           string              `json:"status"`
	Priority         string              `json:"priority"`
	BranchName       string              `json:"branch_name,omitempty"`
	RepoPullRequests []RepoPRSummary     `json:"repo_pull_requests,omitempty"`
	StartedAt        *time.Time          `json:"started_at,omitempty"`
	CompletedAt      *time.Time          `json:"completed_at,omitempty"`
}

// RepoPRSummary is a summary of a pull request for a repository
type RepoPRSummary struct {
	RepositoryName string `json:"repository_name"`
	PRID           string `json:"pr_id"`
	PRURL          string `json:"pr_url,omitempty"`
	PRState        string `json:"pr_state"`
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
	for _, pr := range s.RepoPullRequests {
		result += fmt.Sprintf("\nPR [%s]: %s (state: %s)", pr.RepositoryName, pr.PRID, pr.PRState)
		if pr.PRURL != "" {
			result += fmt.Sprintf(" URL: %s", pr.PRURL)
		}
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
