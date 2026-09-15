package project

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	gomock "go.uber.org/mock/gomock"

	"github.com/stretchr/testify/require"
)

var testPRKey = prReviewKey{repoID: "repo-1", prID: 3}

func TestPRReviews_BeginDedupsSameHead(t *testing.T) {
	p := newPRReviews()
	key := testPRKey

	work := p.begin(key, "sha-a")
	require.NotNil(t, work)
	require.Nil(t, p.begin(key, "sha-a"), "duplicate push for the same head must not start a second review")

	require.False(t, p.release(key, work))

	work = p.begin(key, "sha-a")
	require.NotNil(t, work, "after release, a new push for the same head must be able to review again")
}

func TestPRReviews_NewerPushInterruptsOlderActiveHead(t *testing.T) {
	p := newPRReviews()
	key := testPRKey

	work := p.begin(key, "sha-a")
	require.NotNil(t, work)
	reviewCtx, ok := p.start(key, work, context.Background())
	require.True(t, ok)

	beginAt := time.Now()
	newer := p.begin(key, "sha-b")
	require.NotNil(t, newer)

	// The interrupt must land within 5 seconds of the newer push.
	select {
	case <-reviewCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("newer push did not interrupt the older review within 5 seconds")
	}
	require.Less(t, time.Since(beginAt), 5*time.Second)

	require.True(t, p.release(key, work), "superseded work must be recorded as superseded, not failed")
}

func TestPRReviews_NewerPushRemovesQueuedOlderHeads(t *testing.T) {
	p := newPRReviews()
	key := testPRKey

	queued := p.begin(key, "sha-a")
	require.NotNil(t, queued)
	newer := p.begin(key, "sha-b")
	require.NotNil(t, newer)

	_, ok := p.start(key, queued, context.Background())
	require.False(t, ok, "queued review for an older head must be removed by the newer push")

	reviewCtx, ok := p.start(key, newer, context.Background())
	require.True(t, ok)
	require.NotNil(t, reviewCtx)
	require.NoError(t, reviewCtx.Err())

	require.False(t, p.release(key, newer))
}

func TestPRReviews_IsolatesOtherPRs(t *testing.T) {
	p := newPRReviews()
	sameRepoOtherPR := prReviewKey{repoID: "repo-1", prID: 4}
	otherRepo := prReviewKey{repoID: "repo-2", prID: 3}

	work := p.begin(testPRKey, "sha-a")
	require.NotNil(t, work)
	reviewCtx, ok := p.start(testPRKey, work, context.Background())
	require.True(t, ok)

	require.NotNil(t, p.begin(sameRepoOtherPR, "sha-b"))
	require.NotNil(t, p.begin(otherRepo, "sha-c"))

	// Pushes for other PRs must not interrupt or remove this PR's work.
	select {
	case <-reviewCtx.Done():
		t.Fatal("push for another PR interrupted this PR's review")
	case <-time.After(50 * time.Millisecond):
	}

	require.False(t, p.release(testPRKey, work))
}

func TestHelixCodeReviewTrigger_SupersedesActiveReview(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockController := NewMockController(ctrl)

	trigger := New(&config.ServerConfig{}, mockStore, mockController)
	key := testPRKey

	work := trigger.reviews.begin(key, "sha-a")
	require.NotNil(t, work)

	project := &types.Project{
		ID:                            "test-project-id",
		PullRequestReviewsEnabled:     true,
		PullRequestReviewerHelixAppID: "test-app-id",
	}
	specTask := &types.SpecTask{ID: "test-spec-task-id", Name: "Test Task"}

	app := &types.App{
		ID:             "test-app-id",
		Owner:          "test-owner-id",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "test-org-id",
	}
	user := &types.User{ID: "test-owner-id"}

	mockStore.EXPECT().GetApp(gomock.Any(), "test-app-id").Return(app, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: app.Owner}).Return(user, nil)
	mockController.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)

	// The review session blocks until the newer push cancels it.
	interrupted := make(chan struct{})
	mockController.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *controller.RunSessionRequest) (*types.Interaction, error) {
			<-ctx.Done()
			close(interrupted)
			return nil, ctx.Err()
		},
	)

	go func() {
		time.Sleep(20 * time.Millisecond)
		trigger.reviews.begin(key, "sha-b")
	}()

	start := time.Now()
	err := trigger.runReviewSession(context.Background(), project, specTask, key, work, "sha-a")

	require.NoError(t, err, "superseded review must be recorded as superseded, not failed")
	require.Less(t, time.Since(start), 5*time.Second, "review was not interrupted within 5 seconds")

	select {
	case <-interrupted:
	default:
		t.Fatal("review session was not interrupted")
	}
}

func TestHelixCodeReviewTrigger_DropsQueuedReviewForSupersededHead(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockController := NewMockController(ctrl)

	trigger := New(&config.ServerConfig{}, mockStore, mockController)
	key := testPRKey

	// The review for the older head is queued (registered, session not yet
	// started) when a newer push for the same PR arrives...
	work := trigger.reviews.begin(key, "sha-a")
	require.NotNil(t, work)
	require.NotNil(t, trigger.reviews.begin(key, "sha-b"))

	project := &types.Project{
		ID:                            "test-project-id",
		PullRequestReviewsEnabled:     true,
		PullRequestReviewerHelixAppID: "test-app-id",
	}
	specTask := &types.SpecTask{ID: "test-spec-task-id", Name: "Test Task"}

	// ...so the queued review is removed: it must never create a session or
	// submit comments or a verdict for its superseded head.
	err := trigger.runReviewSession(context.Background(), project, specTask, key, work, "sha-a")
	require.NoError(t, err)
}

func TestHelixCodeReviewTrigger_CompletesUnsupersededReview(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockController := NewMockController(ctrl)

	trigger := New(&config.ServerConfig{}, mockStore, mockController)
	key := testPRKey

	work := trigger.reviews.begin(key, "sha-a")
	require.NotNil(t, work)

	project := &types.Project{
		ID:                            "test-project-id",
		PullRequestReviewsEnabled:     true,
		PullRequestReviewerHelixAppID: "test-app-id",
	}
	specTask := &types.SpecTask{ID: "test-spec-task-id", Name: "Test Task"}

	app := &types.App{
		ID:             "test-app-id",
		Owner:          "test-owner-id",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "test-org-id",
	}
	user := &types.User{ID: "test-owner-id"}

	mockStore.EXPECT().GetApp(gomock.Any(), "test-app-id").Return(app, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: app.Owner}).Return(user, nil)
	mockController.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	mockController.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).Return(&types.Interaction{
		ResponseMessage: "Review completed",
	}, nil)

	err := trigger.runReviewSession(context.Background(), project, specTask, key, work, "sha-a")
	require.NoError(t, err)
}
