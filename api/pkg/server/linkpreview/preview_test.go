package linkpreview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

type fakeStore struct {
	tasks    map[string]*types.SpecTask
	projects map[string]*types.Project
}

func (f *fakeStore) GetSpecTask(_ context.Context, id string) (*types.SpecTask, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	if p, ok := f.projects[id]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func newFake() *fakeStore {
	return &fakeStore{
		tasks: map[string]*types.SpecTask{
			"spt_a": {
				ID: "spt_a", ProjectID: "prj_a", OrganizationID: "org_secret", AssigneeID: "usr_secret",
				Name: "long internal name", ShortTitle: `Fix <script>"x"</script> & stuff`,
				Description: "SECRET PROMPT", TaskNumber: 42, Status: types.TaskStatusPullRequest,
				UpdatedAt: time.Unix(1700000000, 0),
				RepoPullRequests: []types.RepoPR{
					{PRState: "open", RepositoryName: "secret-repo", PRURL: "https://github.com/acme/secret-repo/pull/1"},
					{PRState: "merged"}, {PRState: "merged"}, {PRState: "closed"},
				},
			},
			"spt_archived": {ID: "spt_archived", ProjectID: "prj_a", Name: "gone", Archived: true},
		},
		projects: map[string]*types.Project{"prj_a": {ID: "prj_a", Name: "Acme Web"}},
	}
}

func TestMatchTaskRoute(t *testing.T) {
	cases := []struct {
		path, project, task string
		ok                  bool
	}{
		{"/orgs/acme/projects/prj_a/tasks/spt_a", "prj_a", "spt_a", true},
		{"/orgs/acme/projects/prj_a/tasks/spt_a/", "prj_a", "spt_a", true},
		{"/orgs/acme/projects/prj_a/tasks/spt_a/review/rev_1", "prj_a", "spt_a", true},
		{"/orgs/acme/chat/projects/prj_a/tasks/spt_a", "prj_a", "spt_a", true},
		{"/orgs/acme/projects/prj_a", "", "", false},
		{"/orgs/acme/projects/prj_a/tasks", "", "", false},
		{"/orgs/acme/projects/prj_a/tasks/spt_a/other", "", "", false},
		{"/orgs/acme/projects/prj_a/tasks/spt_a/review/rev_1/x", "", "", false},
		{"/orgs/acme/chat/projects/prj_a/tasks/spt_a/review/rev_1", "", "", false},
		{"/api/v1/spec-tasks/spt_a", "", "", false},
	}
	for _, c := range cases {
		p, task, ok := MatchTaskRoute(c.path)
		if p != c.project || task != c.task || ok != c.ok {
			t.Errorf("%s: got (%q,%q,%v)", c.path, p, task, ok)
		}
	}
}

func TestBuildAndMetaTags(t *testing.T) {
	p, err := Build(context.Background(), newFake(), "spt_a", "prj_a")
	if err != nil {
		t.Fatal(err)
	}
	if p.PRTotal != 4 || p.PROpen != 1 || p.PRMerged != 2 {
		t.Fatalf("pr counts = %d/%d/%d", p.PRTotal, p.PROpen, p.PRMerged)
	}
	tags := MetaTags(p, "https://helix.example/", "/orgs/acme/projects/prj_a/tasks/spt_a")
	for _, want := range []string{
		`<meta property="og:title" content="#42 Fix &lt;script&gt;&#34;x&#34;&lt;/script&gt; &amp; stuff" />`,
		`<meta property="og:description" content="Pull Request · 4 PRs · 1 open · 2 merged · Acme Web" />`,
		`<meta property="og:image" content="https://helix.example/api/v1/public/spec-tasks/spt_a/preview.png?v=1700000000" />`,
		`<meta property="og:url" content="https://helix.example/orgs/acme/projects/prj_a/tasks/spt_a" />`,
		`<meta name="twitter:card" content="summary_large_image" />`,
	} {
		if !strings.Contains(tags, want) {
			t.Errorf("missing %s in\n%s", want, tags)
		}
	}
	for _, secret := range []string{"SECRET", "secret", "org_", "usr_", "github.com", "long internal name"} {
		if strings.Contains(tags, secret) {
			t.Errorf("tags leak %q:\n%s", secret, tags)
		}
	}
}

func TestBuildRefusesHiddenTasks(t *testing.T) {
	s := newFake()
	for _, c := range []struct{ task, project string }{
		{"spt_missing", ""},
		{"spt_archived", ""},
		{"spt_a", "prj_other"},
	} {
		if _, err := Build(context.Background(), s, c.task, c.project); err == nil {
			t.Errorf("Build(%s, %s) succeeded", c.task, c.project)
		}
	}
	if _, err := Build(context.Background(), s, "spt_a", ""); err != nil {
		t.Errorf("image lookup without project: %v", err)
	}
}

func TestTitleTruncation(t *testing.T) {
	p := &Preview{Title: strings.Repeat("é", 200)}
	if n := len([]rune(p.HeadingTitle())); n != maxTitleRunes {
		t.Fatalf("len = %d", n)
	}
}
