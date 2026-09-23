package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRequestDesktopWakeDedupesAndAllowsRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Controller: &controller.Controller{
			Options: controller.Options{Store: mockStore},
		},
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	var calls atomic.Int32
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").DoAndReturn(
		func(context.Context, string) (*types.Session, error) {
			calls.Add(1)
			started <- struct{}{}
			<-release
			return nil, errors.New("desktop unavailable")
		},
	).Times(2)

	server.requestDesktopWake("ses_test")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first desktop wake did not start")
	}

	server.requestDesktopWake("ses_test")
	require.Equal(t, int32(1), calls.Load())

	release <- struct{}{}
	require.Eventually(t, func() bool {
		_, inflight := server.desktopWakeInflight.Load("ses_test")
		return !inflight
	}, time.Second, time.Millisecond)

	server.requestDesktopWake("ses_test")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("retry desktop wake did not start")
	}
	release <- struct{}{}
	require.Equal(t, int32(2), calls.Load())
	require.Eventually(t, func() bool {
		_, inflight := server.desktopWakeInflight.Load("ses_test")
		return !inflight
	}, time.Second, time.Millisecond)
}
