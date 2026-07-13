package helix

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeSpecTaskStore is an in-memory SpecTaskStore for the spectasks
// unit tests. The real impl is *helixstore.Store, satisfied structurally.
type fakeSpecTaskStore struct {
	tasks       map[string]*types.SpecTask
	projects    map[string]*types.Project
	nextTaskNum int
	created     []*types.SpecTask
	updated     []*types.SpecTask
}

func newFakeSpecTaskStore() *fakeSpecTaskStore {
	return &fakeSpecTaskStore{
		tasks:    map[string]*types.SpecTask{},
		projects: map[string]*types.Project{},
	}
}

func (f *fakeSpecTaskStore) CreateSpecTask(_ context.Context, task *types.SpecTask) error {
	f.tasks[task.ID] = task
	f.created = append(f.created, task)
	return nil
}
func (f *fakeSpecTaskStore) GetSpecTask(_ context.Context, id string) (*types.SpecTask, error) {
	t, ok := f.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}
func (f *fakeSpecTaskStore) ListSpecTasks(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	var out []*types.SpecTask
	for _, t := range f.tasks {
		if filters.ProjectID != "" && t.ProjectID != filters.ProjectID {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
func (f *fakeSpecTaskStore) UpdateSpecTask(_ context.Context, task *types.SpecTask) error {
	f.tasks[task.ID] = task
	f.updated = append(f.updated, task)
	return nil
}
func (f *fakeSpecTaskStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		return nil, errors.New("project not found")
	}
	return p, nil
}
func (f *fakeSpecTaskStore) IncrementGlobalTaskNumber(_ context.Context) (int, error) {
	f.nextTaskNum++
	return f.nextTaskNum, nil
}

// fakeSpecTaskWorkflow records the service-level calls.
type fakeSpecTaskWorkflow struct {
	approveCalls        []string
	ensureCalls         []string
	ensurePrimaryID     string
	ensureUserID        string
	requestChangesCalls []string
	lastComment         string
	lastRequestUserID   string
	stopCalls           []string
}

func (f *fakeSpecTaskWorkflow) ApproveSpecs(_ context.Context, task *types.SpecTask) error {
	f.approveCalls = append(f.approveCalls, task.ID)
	return nil
}
func (f *fakeSpecTaskWorkflow) EnsurePullRequests(_ context.Context, task *types.SpecTask, primaryRepoID, userID string) error {
	f.ensureCalls = append(f.ensureCalls, task.ID)
	f.ensurePrimaryID = primaryRepoID
	f.ensureUserID = userID
	// Simulate the system opening a PR.
	task.RepoPullRequests = []types.RepoPR{{RepositoryName: "helix", PRURL: "https://example/pr/1", PRState: "open"}}
	return nil
}
func (f *fakeSpecTaskWorkflow) RequestChanges(_ context.Context, task *types.SpecTask, comment, userID string) error {
	f.requestChangesCalls = append(f.requestChangesCalls, task.ID)
	f.lastComment = comment
	f.lastRequestUserID = userID
	return nil
}
func (f *fakeSpecTaskWorkflow) StopAgent(_ context.Context, task *types.SpecTask) error {
	f.stopCalls = append(f.stopCalls, task.ID)
	return nil
}

func newSpecTasksTestStore(t *testing.T) *storeWrapper {
	t.Helper()
	return newOrgTestStoreForProjectConfig(t)
}

// TestSpecTasks_RejectsNilDeps pins the construction contract.
func TestSpecTasks_RejectsNilDeps(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	if _, err := NewSpecTasks(nil, newFakeSpecTaskStore(), &fakeSpecTaskWorkflow{}); err == nil {
		t.Error("expected error on nil org store")
	}
	if _, err := NewSpecTasks(&wrap.Store, nil, &fakeSpecTaskWorkflow{}); err == nil {
		t.Error("expected error on nil spec task store")
	}
	if _, err := NewSpecTasks(&wrap.Store, newFakeSpecTaskStore(), nil); err == nil {
		t.Error("expected error on nil workflow")
	}
}

// TestSpecTasks_NoProjectReturnsUnsupported pins that a Worker with no
// project pointer gets ErrSpecTasksUnsupported.
func TestSpecTasks_NoProjectReturnsUnsupported(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	st, err := NewSpecTasks(&wrap.Store, newFakeSpecTaskStore(), &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	_, err = st.Create(context.Background(), "org-test", orgchart.BotID("w-noproject"), "", runtime.CreateSpecTaskInput{Name: "x", Description: "y"})
	if !errors.Is(err, runtime.ErrSpecTasksUnsupported) {
		t.Errorf("err = %v, want ErrSpecTasksUnsupported", err)
	}
}

// TestSpecTasks_CreateInOwnProject pins the create path: a task is
// created in the worker's project with a task number, design-doc path,
// and backlog status.
func TestSpecTasks_CreateInOwnProject(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_01abc", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.projects["prj_01abc"] = &types.Project{ID: "prj_01abc", OrganizationID: "org-test"}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.Create(context.Background(), "org-test", wid, "", runtime.CreateSpecTaskInput{
		Name: "Add login", Description: "Add a login page",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Name != "Add login" {
		t.Errorf("Name = %q, want %q", view.Name, "Add login")
	}
	if view.Status != string(types.TaskStatusBacklog) {
		t.Errorf("Status = %q, want backlog", view.Status)
	}
	if len(fs.created) != 1 {
		t.Fatalf("created %d tasks, want 1", len(fs.created))
	}
	got := fs.created[0]
	if got.ProjectID != "prj_01abc" {
		t.Errorf("ProjectID = %q, want prj_01abc", got.ProjectID)
	}
	if got.TaskNumber == 0 || got.DesignDocPath == "" {
		t.Errorf("expected task number + design doc path, got %d / %q", got.TaskNumber, got.DesignDocPath)
	}
}

// TestSpecTasks_GetForeignTaskRejected pins ownership enforcement: a task
// in another project is not readable.
func TestSpecTasks_GetForeignTaskRejected(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.tasks["task_other"] = &types.SpecTask{ID: "task_other", ProjectID: "prj_other"}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	if _, err := st.Get(context.Background(), "org-test", wid, "", "task_other"); err == nil {
		t.Error("expected ownership error for foreign task")
	}
}

// TestSpecTasks_CrossProjectSameOrgAllowed pins the org-wide PM path: a
// Worker whose own project is prj_mine can act on a task in another
// project (prj_other) in the SAME org by passing that project_id.
func TestSpecTasks_CrossProjectSameOrgAllowed(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-pm")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.projects["prj_other"] = &types.Project{ID: "prj_other", OrganizationID: "org-test"}
	fs.tasks["task_o"] = &types.SpecTask{ID: "task_o", ProjectID: "prj_other", Status: types.TaskStatusBacklog}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.Get(context.Background(), "org-test", wid, "prj_other", "task_o")
	if err != nil {
		t.Fatalf("cross-project Get in same org should succeed: %v", err)
	}
	if view.ID != "task_o" {
		t.Errorf("view.ID = %q, want task_o", view.ID)
	}
}

// TestSpecTasks_CrossOrgProjectRejected pins the hard cross-org block: a
// Worker cannot target a project that belongs to another org, even with a
// valid project_id.
func TestSpecTasks_CrossOrgProjectRejected(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-pm")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	// Project exists but belongs to a DIFFERENT org.
	fs.projects["prj_foreign"] = &types.Project{ID: "prj_foreign", OrganizationID: "org-other"}
	fs.tasks["task_f"] = &types.SpecTask{ID: "task_f", ProjectID: "prj_foreign"}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	if _, err := st.Get(context.Background(), "org-test", wid, "prj_foreign", "task_f"); err == nil {
		t.Error("expected cross-org rejection when targeting another org's project")
	}
}

func TestSpecTasks_UpdateMetadata(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", Name: "Old", Description: "Old description", Priority: types.SpecTaskPriorityMedium}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	name, priority, skip := "New", "high", true
	view, err := st.Update(context.Background(), "org-test", wid, "", "task_1", runtime.UpdateSpecTaskInput{
		Name: &name, Priority: &priority, SkipPlanning: &skip,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if view.Name != "New" || view.Priority != "high" || !fs.tasks["task_1"].JustDoItMode {
		t.Errorf("updated task = %+v, skip=%v", view, fs.tasks["task_1"].JustDoItMode)
	}
}

func TestSpecTasks_StopAgentDelegates(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", PlanningSessionID: "ses_task"}
	wf := &fakeSpecTaskWorkflow{}
	st, err := NewSpecTasks(&wrap.Store, fs, wf)
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	if _, err := st.StopAgent(context.Background(), "org-test", wid, "", "task_1"); err != nil {
		t.Fatalf("StopAgent: %v", err)
	}
	if len(wf.stopCalls) != 1 || wf.stopCalls[0] != "task_1" {
		t.Errorf("stop calls = %v", wf.stopCalls)
	}
}

// TestSpecTasks_RequestChangesDeliversComment pins that the reviewer's
// comment is delivered to the agent via the workflow (not dropped), and the
// task transitions to spec_revision with a bumped count.
func TestSpecTasks_RequestChangesDeliversComment(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")
	if err := SaveHiringUser(context.Background(), &wrap.Store, "org-test", wid, "user_hiring"); err != nil {
		t.Fatalf("SaveHiringUser: %v", err)
	}

	fs := newFakeSpecTaskStore()
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", Status: types.TaskStatusSpecReview}
	wf := &fakeSpecTaskWorkflow{}
	st, err := NewSpecTasks(&wrap.Store, fs, wf)
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	if _, err := st.RequestChanges(context.Background(), "org-test", wid, "", "task_1", "tighten scope"); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if len(wf.requestChangesCalls) != 1 || wf.lastComment != "tighten scope" {
		t.Errorf("workflow RequestChanges calls=%v comment=%q, want 1 call with the comment", wf.requestChangesCalls, wf.lastComment)
	}
	if wf.lastRequestUserID != "user_hiring" {
		t.Errorf("actor = %q, want user_hiring", wf.lastRequestUserID)
	}
	got := fs.tasks["task_1"]
	if got.Status != types.TaskStatusSpecRevision || got.SpecRevisionCount != 1 {
		t.Errorf("status=%q count=%d, want spec_revision / 1", got.Status, got.SpecRevisionCount)
	}
}

// TestSpecTasks_ApproveSpecSetsApproverAndDelegates pins that ApproveSpec
// stamps the hiring user as approver and calls the workflow service.
func TestSpecTasks_ApproveSpecSetsApproverAndDelegates(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")
	if err := SaveHiringUser(context.Background(), &wrap.Store, "org-test", wid, "user_hiring"); err != nil {
		t.Fatalf("SaveHiringUser: %v", err)
	}

	fs := newFakeSpecTaskStore()
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", Status: types.TaskStatusSpecReview}
	wf := &fakeSpecTaskWorkflow{}
	st, err := NewSpecTasks(&wrap.Store, fs, wf)
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	if _, err := st.ApproveSpec(context.Background(), "org-test", wid, "", "task_1"); err != nil {
		t.Fatalf("ApproveSpec: %v", err)
	}
	if len(wf.approveCalls) != 1 {
		t.Fatalf("ApproveSpecs called %d times, want 1", len(wf.approveCalls))
	}
	if fs.tasks["task_1"].SpecApprovedBy != "user_hiring" {
		t.Errorf("SpecApprovedBy = %q, want user_hiring", fs.tasks["task_1"].SpecApprovedBy)
	}
}

// TestSpecTasks_RequestChangesTransitions pins RequestChanges → spec_revision.
func TestSpecTasks_RequestChangesTransitions(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", Status: types.TaskStatusSpecReview}
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.RequestChanges(context.Background(), "org-test", wid, "", "task_1", "tighten scope")
	if err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if view.Status != string(types.TaskStatusSpecRevision) {
		t.Errorf("Status = %q, want spec_revision", view.Status)
	}
}

// TestSpecTasks_CreatePullRequestsDelegatesAndMapsPRs pins that the verb
// calls EnsurePullRequests with the project's repo + hiring user and maps
// the resulting PRs into the view.
func TestSpecTasks_CreatePullRequestsDelegatesAndMapsPRs(t *testing.T) {
	t.Parallel()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_mine", "app_x", "repo_y", "ses_z")
	if err := SaveHiringUser(context.Background(), &wrap.Store, "org-test", wid, "user_hiring"); err != nil {
		t.Fatalf("SaveHiringUser: %v", err)
	}

	fs := newFakeSpecTaskStore()
	fs.projects["prj_mine"] = &types.Project{ID: "prj_mine", DefaultRepoID: "repo_primary"}
	fs.tasks["task_1"] = &types.SpecTask{ID: "task_1", ProjectID: "prj_mine", Status: types.TaskStatusImplementationReview}
	wf := &fakeSpecTaskWorkflow{}
	st, err := NewSpecTasks(&wrap.Store, fs, wf)
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.CreatePullRequests(context.Background(), "org-test", wid, "", "task_1")
	if err != nil {
		t.Fatalf("CreatePullRequests: %v", err)
	}
	if len(wf.ensureCalls) != 1 {
		t.Fatalf("EnsurePullRequests called %d times, want 1", len(wf.ensureCalls))
	}
	if wf.ensurePrimaryID != "repo_primary" || wf.ensureUserID != "user_hiring" {
		t.Errorf("EnsurePullRequests args = (%q, %q), want (repo_primary, user_hiring)", wf.ensurePrimaryID, wf.ensureUserID)
	}
	if len(view.PullRequests) != 1 {
		t.Errorf("view PRs = %d, want 1", len(view.PullRequests))
	}
}
