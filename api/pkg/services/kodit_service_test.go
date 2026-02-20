package services

import (
	"testing"
)

func TestKoditService_DisabledWithNilClient(t *testing.T) {
	service := NewKoditService(nil)

	if service.IsEnabled() {
		t.Error("Expected service to be disabled when client is nil")
	}
}

func TestKoditService_RegisterRepositoryWhenDisabled(t *testing.T) {
	service := NewKoditService(nil)

	id, isNew, err := service.RegisterRepository(t.Context(), "https://github.com/example/repo.git")
	if err != nil {
		t.Fatalf("Expected no error for disabled service, got: %v", err)
	}
	if id != 0 {
		t.Errorf("Expected zero ID for disabled service, got: %d", id)
	}
	if isNew {
		t.Error("Expected isNew=false for disabled service")
	}
}

func TestKoditService_SearchSnippetsEmptyQuery(t *testing.T) {
	service := NewKoditService(nil)
	service.enabled = true // Enable but with nil client to test early returns

	results, err := service.SearchSnippets(t.Context(), 1, "", 20)
	if err != nil {
		t.Fatalf("Expected no error for empty query, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results for empty query, got: %d", len(results))
	}
}
