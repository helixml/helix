package memory

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// GGUF magic constants
const (
	GGUF_MAGIC_LE = 0x46554747 // "GGUF" in little endian
	GGUF_MAGIC_BE = 0x47475546 // "GGUF" in big endian
)

// GGUF value types
const (
	GGUF_TYPE_UINT8   = 0
	GGUF_TYPE_INT8    = 1
	GGUF_TYPE_UINT16  = 2
	GGUF_TYPE_INT16   = 3
	GGUF_TYPE_UINT32  = 4
	GGUF_TYPE_INT32   = 5
	GGUF_TYPE_FLOAT32 = 6
	GGUF_TYPE_BOOL    = 7
	GGUF_TYPE_STRING  = 8
	GGUF_TYPE_ARRAY   = 9
	GGUF_TYPE_UINT64  = 10
	GGUF_TYPE_INT64   = 11
	GGUF_TYPE_FLOAT64 = 12
)

// LoadModelMetadata loads and parses GGUF metadata from a model file
func LoadModelMetadata(modelPath string) (*ModelMetadata, error) {
	file, err := os.Open(modelPath)
	if err != nil {
		return nil, &EstimationError{
			Type:    "file_error",
			Message: "failed to open model file",
			Details: err.Error(),
		}
	}
	defer file.Close()

	metadata, err := parseGGUFMetadata(file)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// parseGGUFMetadata parses GGUF format and extracts metadata
func parseGGUFMetadata(r io.ReadSeeker) (*ModelMetadata, error) {
	// Read and verify magic
	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return nil, &EstimationError{
			Type:    "parse_error",
			Message: "failed to read GGUF magic",
			Details: err.Error(),
		}
	}

	var byteOrder binary.ByteOrder
	switch magic {
	case GGUF_MAGIC_LE:
		byteOrder = binary.LittleEndian
	case GGUF_MAGIC_BE:
		byteOrder = binary.BigEndian
	default:
		return nil, &EstimationError{
			Type:    "invalid_format",
			Message: "invalid GGUF magic number",
			Details: fmt.Sprintf("expected 0x%x or 0x%x, got 0x%x", GGUF_MAGIC_LE, GGUF_MAGIC_BE, magic),
		}
	}

	// Read version
	var version uint32
	if err := binary.Read(r, byteOrder, &version); err != nil {
		return nil, &EstimationError{
			Type:    "parse_error",
			Message: "failed to read GGUF version",
			Details: err.Error(),
		}
	}

	// Read tensor count and KV count
	var tensorCount, kvCount uint64
	if err := binary.Read(r, byteOrder, &tensorCount); err != nil {
		return nil, &EstimationError{
			Type:    "parse_error",
			Message: "failed to read tensor count",
			Details: err.Error(),
		}
	}
	if err := binary.Read(r, byteOrder, &kvCount); err != nil {
		return nil, &EstimationError{
			Type:    "parse_error",
			Message: "failed to read KV count",
			Details: err.Error(),
		}
	}

	// Parse KV pairs
	kvPairs := make(map[string]interface{})
	for i := uint64(0); i < kvCount; i++ {
		key, value, err := parseKVPair(r, byteOrder)
		if err != nil {
			return nil, &EstimationError{
				Type:    "parse_error",
				Message: "failed to parse KV pair",
				Details: err.Error(),
			}
		}
		kvPairs[key] = value
	}

	// Parse tensor info (we only need shapes and types for memory estimation)
	layers := make(map[string]LayerInfo)
	for i := uint64(0); i < tensorCount; i++ {
		tensorName, tensorInfo, err := parseTensorInfo(r, byteOrder)
		if err != nil {
			return nil, &EstimationError{
				Type:    "parse_error",
				Message: "failed to parse tensor info",
				Details: err.Error(),
			}
		}

		// Group tensors by layer
		layerName := extractLayerName(tensorName)
		if _, exists := layers[layerName]; !exists {
			layers[layerName] = LayerInfo{Tensors: make(map[string]TensorInfo)}
		}

		tensorKey := strings.TrimPrefix(tensorName, layerName+".")
		if tensorKey == tensorName {
			tensorKey = tensorName // If no prefix was removed, use full name
		}

		layers[layerName].Tensors[tensorKey] = *tensorInfo
	}

	// Extract metadata from KV pairs
	metadata := &ModelMetadata{
		Architecture:    getString(kvPairs, "general.architecture", "unknown"),
		FileType:        getString(kvPairs, "general.file_type", "unknown"),
		BlockCount:      getUint64(kvPairs, "block_count"),
		EmbeddingLength: getUint64(kvPairs, "embedding_length"),
		ContextLength:   getUint64(kvPairs, "context_length"),
		HeadCount:       getUint64(kvPairs, "attention.head_count"),
		HeadCountKV:     getUint64(kvPairs, "attention.head_count_kv"),
		KeyLength:       getUint64(kvPairs, "attention.key_length"),
		ValueLength:     getUint64(kvPairs, "attention.value_length"),
		FFLength:        getUint64(kvPairs, "feed_forward_length"),
		VocabSize:       getVocabSize(kvPairs),
		Layers:          layers,
		AdditionalKV:    kvPairs,
	}

	// Architecture-specific adjustments
	adjustArchitectureSpecificMetadata(metadata, kvPairs)

	return metadata, nil
}

// parseKVPair parses a single key-value pair from GGUF
func parseKVPair(r io.Reader, byteOrder binary.ByteOrder) (string, interface{}, error) {
	// Read key
	key, err := readString(r, byteOrder)
	if err != nil {
		return "", nil, err
	}

	// Read value type
	var valueType uint32
	if err := binary.Read(r, byteOrder, &valueType); err != nil {
		return "", nil, err
	}

	// Read value based on type
	value, err := readValue(r, byteOrder, valueType)
	if err != nil {
		return "", nil, err
	}

	return key, value, nil
}

// parseTensorInfo parses tensor information from GGUF
func parseTensorInfo(r io.Reader, byteOrder binary.ByteOrder) (string, *TensorInfo, error) {
	// Read tensor name
	name, err := readString(r, byteOrder)
	if err != nil {
		return "", nil, err
	}

	// Read number of dimensions
	var nDims uint32
	if err := binary.Read(r, byteOrder, &nDims); err != nil {
		return "", nil, err
	}

	// Read shape
	shape := make([]uint64, nDims)
	for i := uint32(0); i < nDims; i++ {
		if err := binary.Read(r, byteOrder, &shape[i]); err != nil {
			return "", nil, err
		}
	}

	// Read tensor type
	var tensorType uint32
	if err := binary.Read(r, byteOrder, &tensorType); err != nil {
		return "", nil, err
	}

	// Read offset (we don't need this for memory estimation)
	var offset uint64
	if err := binary.Read(r, byteOrder, &offset); err != nil {
		return "", nil, err
	}

	// Calculate tensor size
	size := calculateTensorSize(shape, tensorType)

	tensorInfo := &TensorInfo{
		Shape: shape,
		Type:  tensorTypeToString(tensorType),
		Size:  size,
	}

	return name, tensorInfo, nil
}

// Helper functions for reading GGUF values
func readString(r io.Reader, byteOrder binary.ByteOrder) (string, error) {
	var length uint64
	if err := binary.Read(r, byteOrder, &length); err != nil {
		return "", err
	}

	if length == 0 {
		return "", nil
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}

	return string(data), nil
}

func readValue(r io.Reader, byteOrder binary.ByteOrder, valueType uint32) (interface{}, error) {
	switch valueType {
	case GGUF_TYPE_UINT8:
		var v uint8
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_INT8:
		var v int8
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_UINT16:
		var v uint16
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_INT16:
		var v int16
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_UINT32:
		var v uint32
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_INT32:
		var v int32
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_UINT64:
		var v uint64
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_INT64:
		var v int64
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_FLOAT32:
		var v float32
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_FLOAT64:
		var v float64
		err := binary.Read(r, byteOrder, &v)
		return v, err
	case GGUF_TYPE_BOOL:
		var v uint8
		err := binary.Read(r, byteOrder, &v)
		return v != 0, err
	case GGUF_TYPE_STRING:
		return readString(r, byteOrder)
	case GGUF_TYPE_ARRAY:
		return readArray(r, byteOrder)
	default:
		return nil, fmt.Errorf("unsupported value type: %d", valueType)
	}
}

func readArray(r io.Reader, byteOrder binary.ByteOrder) (interface{}, error) {
	// Read array element type
	var elemType uint32
	if err := binary.Read(r, byteOrder, &elemType); err != nil {
		return nil, err
	}

	// Read array length
	var length uint64
	if err := binary.Read(r, byteOrder, &length); err != nil {
		return nil, err
	}

	// For memory estimation, we only need certain arrays (like tokens for vocab size)
	// For large arrays, we can skip reading the actual data and just return the count
	if elemType == GGUF_TYPE_STRING && length > 100000 {
		// This is likely the tokens array - just skip and return count
		for i := uint64(0); i < length; i++ {
			_, err := readString(r, byteOrder)
			if err != nil {
				return nil, err
			}
		}
		return length, nil // Return count instead of actual strings
	}

	// For smaller arrays, read the actual values
	result := make([]interface{}, length)
	for i := uint64(0); i < length; i++ {
		value, err := readValue(r, byteOrder, elemType)
		if err != nil {
			return nil, err
		}
		result[i] = value
	}

	return result, nil
}

// Helper functions for extracting metadata
func getString(kv map[string]interface{}, key, defaultValue string) string {
	if val, ok := kv[key].(string); ok {
		return val
	}

	// Try with architecture prefix
	if arch, ok := kv["general.architecture"].(string); ok {
		prefixedKey := arch + "." + strings.TrimPrefix(key, "general.")
		if val, ok := kv[prefixedKey].(string); ok {
			return val
		}
	}

	return defaultValue
}

func getUint64(kv map[string]interface{}, key string) uint64 {
	// Try direct key first
	if val := convertToUint64(kv[key]); val > 0 {
		return val
	}

	// Try with architecture prefix
	if arch, ok := kv["general.architecture"].(string); ok {
		prefixedKey := arch + "." + key
		if val := convertToUint64(kv[prefixedKey]); val > 0 {
			return val
		}
	}

	return 0
}

func convertToUint64(val interface{}) uint64 {
	switch v := val.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint8:
		return uint64(v)
	case int64:
		if v >= 0 {
			return uint64(v)
		}
	case int32:
		if v >= 0 {
			return uint64(v)
		}
	case int16:
		if v >= 0 {
			return uint64(v)
		}
	case int8:
		if v >= 0 {
			return uint64(v)
		}
	}
	return 0
}

func getVocabSize(kv map[string]interface{}) uint64 {
	// Try to get vocab size from tokens array
	if tokens, ok := kv["tokenizer.ggml.tokens"]; ok {
		switch t := tokens.(type) {
		case uint64:
			return t // This is the count we returned for large arrays
		case []interface{}:
			return uint64(len(t))
		}
	}

	// Fallback: try to find vocab size in other fields
	if val := getUint64(kv, "tokenizer.vocab_size"); val > 0 {
		return val
	}

	return 32000 // Reasonable default
}

// extractLayerName extracts the layer name from a tensor name
func extractLayerName(tensorName string) string {
	// Handle common layer patterns
	if strings.HasPrefix(tensorName, "blk.") {
		parts := strings.Split(tensorName, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1] // e.g., "blk.0"
		}
	}

	// Handle output layers
	if strings.HasPrefix(tensorName, "output") {
		return "output"
	}

	// Handle embedding layers
	if strings.HasPrefix(tensorName, "token_embd") {
		return "token_embd"
	}

	// Handle vision layers
	if strings.HasPrefix(tensorName, "v.") {
		parts := strings.Split(tensorName, ".")
		if len(parts) >= 2 {
			return "v"
		}
	}

	// Default: use the first part before the first dot
	parts := strings.Split(tensorName, ".")
	return parts[0]
}

// calculateTensorSize calculates the size of a tensor in bytes
func calculateTensorSize(shape []uint64, tensorType uint32) uint64 {
	// Calculate number of elements
	elements := uint64(1)
	for _, dim := range shape {
		elements *= dim
	}

	// Get type size and block size
	typeSize := getTensorTypeSize(tensorType)
	blockSize := getTensorBlockSize(tensorType)

	// Calculate total size
	return elements * typeSize / blockSize
}

// getTensorTypeSize returns the size in bytes for a tensor type
func getTensorTypeSize(tensorType uint32) uint64 {
	switch tensorType {
	case 0: // F32
		return 4
	case 1: // F16
		return 2
	case 2: // Q4_0
		return 2 + 32/2 // 2 bytes + 16 4-bit values
	case 3: // Q4_1
		return 2 + 2 + 32/2 // 2 + 2 bytes + 16 4-bit values
	case 6: // Q5_0
		return 2 + 4 + 32/2
	case 7: // Q5_1
		return 2 + 2 + 4 + 32/2
	case 8: // Q8_0
		return 2 + 32
	case 9: // Q8_1
		return 2 + 2 + 32
	case 10: // Q2_K
		return 32/16 + 32/4 + 2 + 2
	case 11: // Q3_K
		return 32/8 + 32/4 + 12 + 2
	case 12: // Q4_K
		return 2 + 2 + 12 + 32/2
	case 13: // Q5_K
		return 2 + 2 + 12 + 32/8 + 32/2
	case 14: // Q6_K
		return 32/2 + 32/4 + 32/16 + 2
	case 15: // Q8_K
		return 4 + 32 + 2*32/16
	case 30: // BF16
		return 2
	default:
		return 4 // Default to F32 size
	}
}

// getTensorBlockSize returns the block size for a tensor type
func getTensorBlockSize(tensorType uint32) uint64 {
	switch tensorType {
	case 0, 1, 24, 25, 26, 27, 28, 30: // F32, F16, I8, I16, I32, I64, F64, BF16
		return 1
	case 2, 3, 6, 7, 8, 9: // Q4_0, Q4_1, Q5_0, Q5_1, Q8_0, Q8_1
		return 32
	default:
		return 256 // K-quants and others
	}
}

// tensorTypeToString converts tensor type ID to string
func tensorTypeToString(tensorType uint32) string {
	switch tensorType {
	case 0:
		return "F32"
	case 1:
		return "F16"
	case 2:
		return "Q4_0"
	case 3:
		return "Q4_1"
	case 6:
		return "Q5_0"
	case 7:
		return "Q5_1"
	case 8:
		return "Q8_0"
	case 9:
		return "Q8_1"
	case 10:
		return "Q2_K"
	case 11:
		return "Q3_K"
	case 12:
		return "Q4_K"
	case 13:
		return "Q5_K"
	case 14:
		return "Q6_K"
	case 15:
		return "Q8_K"
	case 30:
		return "BF16"
	default:
		return fmt.Sprintf("TYPE_%d", tensorType)
	}
}

// adjustArchitectureSpecificMetadata makes architecture-specific adjustments to metadata
func adjustArchitectureSpecificMetadata(metadata *ModelMetadata, kv map[string]interface{}) {
	// Handle architecture-specific fields
	switch metadata.Architecture {
	case "qwen2", "qwen3":
		// Qwen models might have different field names
		if metadata.HeadCountKV == 0 {
			metadata.HeadCountKV = getUint64(kv, "attention.head_count_kv")
		}
	case "gemma", "gemma2", "gemma3":
		// Gemma models might have specific configurations
		if metadata.KeyLength == 0 && metadata.HeadCount > 0 && metadata.EmbeddingLength > 0 {
			metadata.KeyLength = metadata.EmbeddingLength / metadata.HeadCount
		}
		if metadata.ValueLength == 0 {
			metadata.ValueLength = metadata.KeyLength
		}
	case "llama", "llama4":
		// LLaMA models have standard configurations
		if metadata.HeadCountKV == 0 {
			metadata.HeadCountKV = metadata.HeadCount // No GQA in base LLaMA
		}
	}

	// Ensure HeadCountKV is set
	if metadata.HeadCountKV == 0 {
		metadata.HeadCountKV = metadata.HeadCount
	}

	// Ensure Key/Value lengths are set
	if metadata.KeyLength == 0 && metadata.HeadCount > 0 && metadata.EmbeddingLength > 0 {
		metadata.KeyLength = metadata.EmbeddingLength / metadata.HeadCount
	}
	if metadata.ValueLength == 0 {
		metadata.ValueLength = metadata.KeyLength
	}
}
