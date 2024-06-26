// Code generated by MockGen. DO NOT EDIT.
// Source: openai_client.go

// Package openai is a generated GoMock package.
package openai

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	openai "github.com/lukemarsden/go-openai2"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// CreateChatCompletion mocks base method.
func (m *MockClient) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateChatCompletion", ctx, request)
	ret0, _ := ret[0].(openai.ChatCompletionResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateChatCompletion indicates an expected call of CreateChatCompletion.
func (mr *MockClientMockRecorder) CreateChatCompletion(ctx, request interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateChatCompletion", reflect.TypeOf((*MockClient)(nil).CreateChatCompletion), ctx, request)
}
