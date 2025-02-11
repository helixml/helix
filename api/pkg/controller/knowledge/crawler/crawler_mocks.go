// Code generated by MockGen. DO NOT EDIT.
// Source: crawler.go
//
// Generated by this command:
//
//	mockgen -source crawler.go -destination crawler_mocks.go -package crawler
//

// Package crawler is a generated GoMock package.
package crawler

import (
	context "context"
	reflect "reflect"

	types "github.com/helixml/helix/api/pkg/types"
	gomock "go.uber.org/mock/gomock"
)

// MockCrawler is a mock of Crawler interface.
type MockCrawler struct {
	ctrl     *gomock.Controller
	recorder *MockCrawlerMockRecorder
	isgomock struct{}
}

// MockCrawlerMockRecorder is the mock recorder for MockCrawler.
type MockCrawlerMockRecorder struct {
	mock *MockCrawler
}

// NewMockCrawler creates a new mock instance.
func NewMockCrawler(ctrl *gomock.Controller) *MockCrawler {
	mock := &MockCrawler{ctrl: ctrl}
	mock.recorder = &MockCrawlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCrawler) EXPECT() *MockCrawlerMockRecorder {
	return m.recorder
}

// Crawl mocks base method.
func (m *MockCrawler) Crawl(ctx context.Context) ([]*types.CrawledDocument, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Crawl", ctx)
	ret0, _ := ret[0].([]*types.CrawledDocument)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Crawl indicates an expected call of Crawl.
func (mr *MockCrawlerMockRecorder) Crawl(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Crawl", reflect.TypeOf((*MockCrawler)(nil).Crawl), ctx)
}
