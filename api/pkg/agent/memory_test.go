package agent

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDefaultMemory_Retrieve(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	meta := &Meta{
		UserID: "user_id",
		AppID:  "app_id",
	}

	store := store.NewMockStore(ctrl)
	store.EXPECT().ListMemories(gomock.Any(), &types.ListMemoryRequest{
		UserID: meta.UserID,
		AppID:  meta.AppID,
	}).Return([]*types.Memory{
		{
			ID:       "1",
			Contents: "contents1",
		},
		{
			ID:       "2",
			Contents: "contents2",
		},
	}, nil)

	memory := NewDefaultMemory(true, store)
	parsed, err := memory.Retrieve(meta)
	require.NoError(t, err)

	memStr := parsed.Parse()

	// Check that memory contains contents from both
	require.Contains(t, memStr, "contents1")
	require.Contains(t, memStr, "contents2")

}

func TestMemoryBlock_AddString(t *testing.T) {
	mb := NewMemoryBlock()

	// Test adding string value
	mb.AddString("key1", "value1")

	// Verify the item was added correctly
	if !mb.Exists("key1") {
		t.Error("Key 'key1' doesn't exist after adding")
	}

	// Verify the value type is correct
	value := mb.Items["key1"]
	if !value.IsString() {
		t.Error("Value should be a string but isn't")
	}

	// Verify the value content
	if value.AsString() != "value1" {
		t.Errorf("Expected string value 'value1', got '%s'", value.AsString())
	}
}

func TestMemoryBlock_AddBlock(t *testing.T) {
	mb := NewMemoryBlock()
	nested := NewMemoryBlock()

	// Test adding memory block value
	mb.AddBlock("nested", nested)

	// Verify the item was added correctly
	if !mb.Exists("nested") {
		t.Error("Key 'nested' doesn't exist after adding")
	}

	// Verify the value type is correct
	value := mb.Items["nested"]
	if !value.IsBlock() {
		t.Error("Value should be a MemoryBlock but isn't")
	}

	// Verify the value content
	if value.AsBlock() != nested {
		t.Error("Block reference doesn't match the original")
	}
}

func TestMemoryBlock_Delete(t *testing.T) {
	mb := NewMemoryBlock()
	mb.AddString("key1", "value1")

	// Test deleting existing key
	deleted := mb.Delete("key1")
	if !deleted {
		t.Error("Delete reported false when key should exist")
	}

	// Test deleting non-existing key
	deleted = mb.Delete("nonexistent")
	if deleted {
		t.Error("Delete reported true when key shouldn't exist")
	}
}

func TestMemoryBlock_Exists(t *testing.T) {
	mb := NewMemoryBlock()
	mb.AddString("key1", "value1")

	// Test existing key
	exists := mb.Exists("key1")
	if !exists {
		t.Error("Exists reported false when key should exist")
	}

	// Test non-existing key
	exists = mb.Exists("nonexistent")
	if exists {
		t.Error("Exists reported true when key shouldn't exist")
	}
}

func TestMemoryBlock_Parse(t *testing.T) {
	// Create a complex memory block structure for testing
	root := NewMemoryBlock()
	root.AddString("key1", "value1")
	root.AddString("key2", "value2")

	nested1 := NewMemoryBlock()
	nested1.AddString("nestedKey1", "nestedValue1")
	nested1.AddString("nestedKey2", "nestedValue2")

	deeplyNested := NewMemoryBlock()
	deeplyNested.AddString("deepKey", "deepValue")
	nested1.AddBlock("deep", deeplyNested)

	root.AddBlock("nested", nested1)

	// Generate parsed string
	parsed := root.Parse()

	// Verify structure with expected output
	expected := `<Memory>
  key1: value1
  key2: value2
  <nested>
    nestedKey1: nestedValue1
    nestedKey2: nestedValue2
    <deep>
      deepKey: deepValue
    </deep>
  </nested>
</Memory>
`

	// Normalize line endings for comparison
	parsed = strings.ReplaceAll(parsed, "\r\n", "\n")
	expected = strings.ReplaceAll(expected, "\r\n", "\n")

	if parsed != expected {
		t.Errorf("Parse output doesn't match expected.\nGot:\n%s\nExpected:\n%s", parsed, expected)
	}
}
