// Package linkpreview builds the public link previews (Open Graph tags and a
// rendered card image) that Slack and other chat apps show when a spec task
// URL is pasted. Crawlers fetch without cookies, so everything here is
// unauthenticated: only the allow-listed fields in Preview are ever exposed.
package linkpreview

import (
	"context"
	"fmt"
	"html"
	"image/color"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// Store is the subset of the Helix store needed to build a preview.
type Store interface {
	GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error)
	GetProject(ctx context.Context, projectID string) (*types.Project, error)
}

// Preview holds the only task information exposed publicly. Tags and the
// image are both built from it, so nothing else can leak.
type Preview struct {
	TaskID      string
	Number      int
	Title       string
	StatusLabel string
	StatusColor color.RGBA
	Project     string
	PRTotal     int
	PROpen      int
	PRMerged    int
	Version     int64 // task updated_at (unix), used to bust image caches
}

const maxTitleRunes = 80

// Build loads a task and returns its preview. It fails (with no distinction
// between missing and hidden) when the task does not exist, is archived, or
// does not belong to projectID (when projectID is non-empty).
func Build(ctx context.Context, s Store, taskID, projectID string) (*Preview, error) {
	task, err := s.GetSpecTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, fmt.Errorf("no preview for task")
	}
	if task.Archived || (projectID != "" && task.ProjectID != projectID) {
		return nil, fmt.Errorf("no preview for task")
	}
	p := &Preview{
		TaskID:  task.ID,
		Number:  task.TaskNumber,
		Title:   taskTitle(task),
		Version: task.UpdatedAt.Unix(),
	}
	p.StatusLabel, p.StatusColor = statusStyle(task.Status)
	if project, err := s.GetProject(ctx, task.ProjectID); err == nil && project != nil {
		p.Project = project.Name
	}
	for _, pr := range task.RepoPullRequests {
		p.PRTotal++
		switch pr.PRState {
		case "open":
			p.PROpen++
		case "merged":
			p.PRMerged++
		}
	}
	return p, nil
}

func taskTitle(t *types.SpecTask) string {
	for _, s := range []string{t.UserShortTitle, t.ShortTitle, t.Name} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return "Untitled task"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// HeadingTitle is the title as shown in og:title: "#142 Add dark mode".
func (p *Preview) HeadingTitle() string {
	t := truncate(p.Title, maxTitleRunes)
	if p.Number > 0 {
		return fmt.Sprintf("#%d %s", p.Number, t)
	}
	return t
}

// PRSummary is e.g. "3 PRs · 1 open · 2 merged", or "" when there are none.
func (p *Preview) PRSummary() string {
	if p.PRTotal == 0 {
		return ""
	}
	noun := "PRs"
	if p.PRTotal == 1 {
		noun = "PR"
	}
	return fmt.Sprintf("%d %s · %d open · %d merged", p.PRTotal, noun, p.PROpen, p.PRMerged)
}

// Description is the og:description: status, PRs and project.
func (p *Preview) Description() string {
	parts := []string{p.StatusLabel}
	if s := p.PRSummary(); s != "" {
		parts = append(parts, s)
	}
	if p.Project != "" {
		parts = append(parts, p.Project)
	}
	return strings.Join(parts, " · ")
}

// ImagePath is the image URL path, versioned so chat apps refetch on change.
func (p *Preview) ImagePath() string {
	return fmt.Sprintf("/api/v1/public/spec-tasks/%s/preview.png?v=%d", p.TaskID, p.Version)
}

// MetaTags renders the Open Graph and Twitter tags for a task page. baseURL
// is the public server URL (no trailing slash); pagePath is the request path.
func MetaTags(p *Preview, baseURL, pagePath string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	return tags([][2]string{
		{"og:site_name", "Helix"},
		{"og:type", "website"},
		{"og:title", p.HeadingTitle()},
		{"og:description", p.Description()},
		{"og:url", baseURL + pagePath},
		{"og:image", baseURL + p.ImagePath()},
		{"og:image:width", fmt.Sprint(ImageWidth)},
		{"og:image:height", fmt.Sprint(ImageHeight)},
		{"og:image:alt", p.HeadingTitle()},
		{"twitter:card", "summary_large_image"},
		{"twitter:title", p.HeadingTitle()},
		{"twitter:description", p.Description()},
		{"twitter:image", baseURL + p.ImagePath()},
	})
}

func tags(kv [][2]string) string {
	var b strings.Builder
	for _, t := range kv {
		attr := "property"
		if strings.HasPrefix(t[0], "twitter:") {
			attr = "name"
		}
		fmt.Fprintf(&b, "<meta %s=\"%s\" content=\"%s\" />\n", attr, t[0], html.EscapeString(t[1]))
	}
	return b.String()
}

// MatchTaskRoute extracts the project and task IDs from a spec task page path:
//
//	/orgs/{org}/projects/{project}/tasks/{task}[/review/{review}]
//	/orgs/{org}/chat/projects/{project}/tasks/{task}
func MatchTaskRoute(path string) (projectID, taskID string, ok bool) {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	if len(seg) < 6 || seg[0] != "orgs" {
		return "", "", false
	}
	rest := seg[2:]
	if rest[0] == "chat" {
		rest = rest[1:]
		if len(rest) != 4 {
			return "", "", false
		}
	}
	switch {
	case len(rest) == 4, len(rest) == 6 && rest[4] == "review":
	default:
		return "", "", false
	}
	if rest[0] != "projects" || rest[2] != "tasks" || rest[1] == "" || rest[3] == "" {
		return "", "", false
	}
	return rest[1], rest[3], true
}

func statusStyle(s types.SpecTaskStatus) (string, color.RGBA) {
	var (
		grey   = color.RGBA{0x9c, 0xa3, 0xaf, 0xff}
		blue   = color.RGBA{0x60, 0xa5, 0xfa, 0xff}
		amber  = color.RGBA{0xfb, 0xbf, 0x24, 0xff}
		purple = color.RGBA{0xa7, 0x8b, 0xfa, 0xff}
		green  = color.RGBA{0x4a, 0xde, 0x80, 0xff}
		red    = color.RGBA{0xf8, 0x71, 0x71, 0xff}
	)
	switch s {
	case types.TaskStatusBacklog, types.TaskStatusPreparing:
		return "Backlog", grey
	case types.TaskStatusQueuedSpecGeneration, types.TaskStatusSpecGeneration:
		return "Planning", blue
	case types.TaskStatusSpecReview, types.TaskStatusSpecRevision:
		return "Spec Review", amber
	case types.TaskStatusSpecApproved, types.TaskStatusQueuedImplementation,
		types.TaskStatusImplementationQueued, types.TaskStatusImplementation:
		return "Implementation", blue
	case types.TaskStatusImplementationReview:
		return "Implementation Review", amber
	case types.TaskStatusPullRequest:
		return "Pull Request", purple
	case types.TaskStatusDone:
		return "Done", green
	case types.TaskStatusSpecFailed, types.TaskStatusImplementationFailed:
		return "Failed", red
	}
	return "In Progress", grey
}
