// Code generated by MockGen. DO NOT EDIT.
// Source: observability.go
//
// Generated by this command:
//
//	mockgen -source observability.go -destination observability_mocks.go -package agent
//

// Package agent is a generated GoMock package.
package agent

import (
	context "context"
	reflect "reflect"

	types "github.com/helixml/helix/api/pkg/types"
	gomock "go.uber.org/mock/gomock"
)

// MockStepInfoEmitter is a mock of StepInfoEmitter interface.
type MockStepInfoEmitter struct {
	ctrl     *gomock.Controller
	recorder *MockStepInfoEmitterMockRecorder
	isgomock struct{}
}

// MockStepInfoEmitterMockRecorder is the mock recorder for MockStepInfoEmitter.
type MockStepInfoEmitterMockRecorder struct {
	mock *MockStepInfoEmitter
}

// NewMockStepInfoEmitter creates a new mock instance.
func NewMockStepInfoEmitter(ctrl *gomock.Controller) *MockStepInfoEmitter {
	mock := &MockStepInfoEmitter{ctrl: ctrl}
	mock.recorder = &MockStepInfoEmitterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStepInfoEmitter) EXPECT() *MockStepInfoEmitterMockRecorder {
	return m.recorder
}

// EmitStepInfo mocks base method.
func (m *MockStepInfoEmitter) EmitStepInfo(ctx context.Context, info *types.StepInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EmitStepInfo", ctx, info)
	ret0, _ := ret[0].(error)
	return ret0
}

// EmitStepInfo indicates an expected call of EmitStepInfo.
func (mr *MockStepInfoEmitterMockRecorder) EmitStepInfo(ctx, info any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EmitStepInfo", reflect.TypeOf((*MockStepInfoEmitter)(nil).EmitStepInfo), ctx, info)
}
