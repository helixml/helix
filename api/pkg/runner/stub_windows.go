//go:build windows
// +build windows

package runner

import (
	"context"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
)

// This file contains stub implementations for Windows builds.
// It ensures that the package can be imported on Windows
// without including any of the Unix-specific code.

// ModelInstanceConfig stub
type ModelInstanceConfig struct {
	InitialSession    *types.Session
	NextTaskURL       string
	InitialSessionURL string
	ResponseHandler   func(res *types.RunnerTaskResponse) error
	GetNextSession    func() (*types.Session, error)
	RunnerOptions     RunnerOptions
}

// RunnerOptions is already defined in controller.go, so we'll remove it from here

// Stub implementation of AxolotlModelInstance
var _ ModelInstance = &AxolotlModelInstance{}

type AxolotlModelInstance struct{}

func (a *AxolotlModelInstance) ID() string                                   { return "" }
func (a *AxolotlModelInstance) Filter() types.SessionFilter                  { return types.SessionFilter{} }
func (a *AxolotlModelInstance) Stale() bool                                  { return false }
func (a *AxolotlModelInstance) Model() model.Model                           { return nil }
func (a *AxolotlModelInstance) NextSession() *types.Session                  { return nil }
func (a *AxolotlModelInstance) SetNextSession(*types.Session)                {}
func (a *AxolotlModelInstance) GetQueuedSession() *types.Session             { return nil }
func (a *AxolotlModelInstance) Done() <-chan bool                            { return nil }
func (a *AxolotlModelInstance) GetState() (*types.ModelInstanceState, error) { return nil, nil }
func (a *AxolotlModelInstance) QueueSession(*types.Session, bool)            {}
func (a *AxolotlModelInstance) Start(context.Context) error                  { return nil }
func (a *AxolotlModelInstance) Stop() error                                  { return nil }
func (a *AxolotlModelInstance) IsActive() bool                               { return false }
func (a *AxolotlModelInstance) AssignSessionTask(context.Context, *types.Session) (*types.RunnerTask, error) {
	return nil, nil
}

// Stub implementation of NewAxolotlModelInstance
func NewAxolotlModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*AxolotlModelInstance, error) {
	return nil, nil
}

// Stub implementation of killProcessTree
func killProcessTree(pid int) error {
	return nil
}

// FreePortFinder interface stub
type FreePortFinder interface {
	GetFreePort() (int, error)
}

// Stub implementation of FreePortFinder
type stubFreePortFinder struct{}

func (s *stubFreePortFinder) GetFreePort() (int, error) {
	return 0, nil
}

var freePortFinder FreePortFinder = &stubFreePortFinder{}

// Add any other necessary stub functions or types here

type CogModelInstance struct{}

func (a *CogModelInstance) ID() string                                   { return "" }
func (a *CogModelInstance) Filter() types.SessionFilter                  { return types.SessionFilter{} }
func (a *CogModelInstance) Stale() bool                                  { return false }
func (a *CogModelInstance) Model() model.Model                           { return nil }
func (a *CogModelInstance) NextSession() *types.Session                  { return nil }
func (a *CogModelInstance) SetNextSession(*types.Session)                {}
func (a *CogModelInstance) GetQueuedSession() *types.Session             { return nil }
func (a *CogModelInstance) Done() <-chan bool                            { return nil }
func (a *CogModelInstance) GetState() (*types.ModelInstanceState, error) { return nil, nil }
func (a *CogModelInstance) QueueSession(*types.Session, bool)            {}
func (a *CogModelInstance) Start(context.Context) error                  { return nil }
func (a *CogModelInstance) Stop() error                                  { return nil }
func (a *CogModelInstance) IsActive() bool                               { return false }
func (a *CogModelInstance) AssignSessionTask(context.Context, *types.Session) (*types.RunnerTask, error) {
	return nil, nil
}
func NewCogModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*CogModelInstance, error) {
	return nil, nil
}
