// Package memory provides an interface for storing and retrieving conversation data.
package agent

import (
	"fmt"
	"strings"
)

// ValueType represents the type of a memory value
type ValueType int

const (
	StringType ValueType = iota
	BlockType
)

// MemoryValue represents a value that can be either a string or a nested MemoryBlock
type MemoryValue struct {
	valueType ValueType
	stringVal string
	blockVal  *MemoryBlock
}

// NewStringValue creates a MemoryValue containing a string
func NewStringValue(s string) MemoryValue {
	return MemoryValue{
		valueType: StringType,
		stringVal: s,
	}
}

// NewBlockValue creates a MemoryValue containing a MemoryBlock
func NewBlockValue(block *MemoryBlock) MemoryValue {
	return MemoryValue{
		valueType: BlockType,
		blockVal:  block,
	}
}

// Type returns the type of the value
func (mv MemoryValue) Type() ValueType {
	return mv.valueType
}

// AsString returns the string value if type is StringType, empty string otherwise
func (mv MemoryValue) AsString() string {
	if mv.valueType == StringType {
		return mv.stringVal
	}
	return ""
}

// AsBlock returns the MemoryBlock value if type is BlockType, nil otherwise
func (mv MemoryValue) AsBlock() *MemoryBlock {
	if mv.valueType == BlockType {
		return mv.blockVal
	}
	return nil
}

// IsString returns true if the value is a string
func (mv MemoryValue) IsString() bool {
	return mv.valueType == StringType
}

// IsBlock returns true if the value is a MemoryBlock
func (mv MemoryValue) IsBlock() bool {
	return mv.valueType == BlockType
}

// MemoryBlock represents a key-value store where values can be strings or nested MemoryBlocks
type MemoryBlock struct {
	Items map[string]MemoryValue // For storing multiple key-value pairs
}

// NewMemoryBlock creates a new MemoryBlock with initialized map
func NewMemoryBlock() *MemoryBlock {
	return &MemoryBlock{
		Items: make(map[string]MemoryValue),
	}
}

// AddString adds a string value for the given key
func (mb *MemoryBlock) AddString(key string, value string) {
	mb.Items[key] = NewStringValue(value)
}

// AddBlock adds a MemoryBlock value for the given key
func (mb *MemoryBlock) AddBlock(key string, value *MemoryBlock) {
	mb.Items[key] = NewBlockValue(value)
}

// Delete removes a key-value pair from the MemoryBlock
// Returns true if the key was found and deleted, false otherwise
func (mb *MemoryBlock) Delete(key string) bool {
	if _, exists := mb.Items[key]; exists {
		delete(mb.Items, key)
		return true
	}
	return false
}

// Exists checks if a key exists in the MemoryBlock
func (mb *MemoryBlock) Exists(key string) bool {
	_, exists := mb.Items[key]
	return exists
}

// Parse generates a string representation of the MemoryBlock
// recursively parsing any nested MemoryBlocks into XML-style format
func (mb *MemoryBlock) Parse() string {
	return mb.parseWithIndent(0, "Memory")
}

// parseWithIndent is a helper method for Parse that handles indentation
func (mb *MemoryBlock) parseWithIndent(level int, tagName string) string {
	var result strings.Builder
	indent := strings.Repeat("  ", level)

	// Open tag
	result.WriteString(fmt.Sprintf("%s<%s>\n", indent, tagName))

	// First process all string values
	for k, v := range mb.Items {
		if v.IsString() {
			innerIndent := strings.Repeat("  ", level+1)
			result.WriteString(fmt.Sprintf("%s%s: %v\n", innerIndent, k, v.AsString()))
		}
	}

	// Then process all nested blocks
	for k, v := range mb.Items {
		if v.IsBlock() {
			result.WriteString(v.AsBlock().parseWithIndent(level+1, k))
		}
	}

	// Close tag
	result.WriteString(fmt.Sprintf("%s</%s>\n", indent, tagName))

	return result.String()
}

// Memory is an interface for reading/writing conversation data or other context.
type Memory interface {
	Retrieve(meta *Meta) (*MemoryBlock, error)
}
