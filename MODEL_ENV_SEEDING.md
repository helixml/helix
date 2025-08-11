# Model Database Seeding from Environment Variables

## Design Approach

Given the complexity of model specifications (runtime, memory, context length, runtime args, etc.), we'll use a structured JSON-based approach with environment variables.

## Environment Variable Patterns

### Option 1: Individual JSON Models (Recommended)
```bash
# Define individual models as JSON strings
HELIX_SEED_MODEL_1='{"id":"llama3.1:8b","name":"Llama 3.1 8B","type":"chat","runtime":"ollama","memory":"8GB","enabled":true}'

HELIX_SEED_MODEL_2='{"id":"Qwen/Qwen2.5-VL-7B-Instruct","name":"Qwen 2.5 VL 7B","type":"chat","runtime":"vllm","memory":"39GB","context_length":32768,"runtime_args":["--trust-remote-code","--max-model-len","32768"]}'

HELIX_SEED_MODEL_3='{"id":"BAAI/bge-large-en-v1.5","name":"BGE Large EN v1.5","type":"embed","runtime":"vllm","memory":"5GiB","context_length":512,"runtime_args":["--task","embed","--trust-remote-code"]}'

# Control seeding behavior
HELIX_SEED_MODELS_ENABLED=true
HELIX_SEED_MODELS_UPDATE_EXISTING=false  # Don't overwrite user-modified models
HELIX_SEED_MODELS_PREFIX="HELIX_SEED_MODEL_"
```

### Option 2: Single JSON Array (Alternative)
```bash
# Define all models in a single JSON array
HELIX_SEED_MODELS='[
  {
    "id": "llama3.1:8b",
    "name": "Llama 3.1 8B", 
    "type": "chat",
    "runtime": "ollama",
    "memory": "8GB",
    "enabled": true
  },
  {
    "id": "Qwen/Qwen2.5-VL-7B-Instruct",
    "name": "Qwen 2.5 VL 7B",
    "type": "chat", 
    "runtime": "vllm",
    "memory": "39GB",
    "context_length": 32768,
    "runtime_args": [
      "--trust-remote-code", 
      "--max-model-len", "32768"
    ]
  }
]'
```

### Option 3: File-Based (Most Flexible)
```bash
# Point to JSON files containing model definitions
HELIX_SEED_MODELS_FILE="/etc/helix/seed-models.json"
HELIX_SEED_MODELS_DIR="/etc/helix/models.d/"  # Directory with *.json files
```

## Implementation Strategy

### Phase 1: Individual JSON Models (Option 1)
- **Pros**: Easy to manage, can be set per-model, clear separation
- **Cons**: Many environment variables for large model sets
- **Use case**: Docker deployments, small model sets

### Phase 2: File-Based (Option 3) 
- **Pros**: Most flexible, supports large model sets, version control friendly
- **Cons**: Requires file system access
- **Use case**: Kubernetes ConfigMaps, large deployments

## Environment Variable Schema

Each model JSON should follow this schema:
```typescript
interface SeedModel {
  // Required fields
  id: string;                    // Model identifier
  name: string;                  // Display name
  type: "chat" | "image" | "embed"; // Model type
  runtime: "ollama" | "vllm" | "diffusers" | "axolotl"; // Runtime
  
  // Optional fields  
  memory?: number | string;      // Memory in bytes (number) or human-readable format (string: "8GB", "16GiB")
  context_length?: number;       // Context window size
  description?: string;          // Model description
  enabled?: boolean;             // Default: true
  hide?: boolean;                // Default: false
  auto_pull?: boolean;           // Default: false  
  prewarm?: boolean;             // Default: false
  user_modified?: boolean;       // Default: false (system managed)
  
  // Runtime-specific configuration (flattened array format supported)
  runtime_args?: string[] | {    // Direct array: ["--trust-remote-code"] or nested: {"args": [...]}
    args?: string[];             // Command line arguments
    [key: string]: any;          // Other runtime-specific config
  };
}
```

## Seeding Behavior

### Default Behavior
1. **Enabled by default**: `HELIX_SEED_MODELS_ENABLED=true`
2. **Respect user modifications**: Don't overwrite `user_modified=true` models
3. **Additive**: Add new models, don't remove existing ones
4. **Startup only**: Run seeding during server startup

### Configuration Options
```bash
# Enable/disable seeding
HELIX_SEED_MODELS_ENABLED=true

# Update existing system models (user_modified=false)
HELIX_SEED_MODELS_UPDATE_EXISTING=true

# Force update all models (dangerous)
HELIX_SEED_MODELS_FORCE_UPDATE=false

# Environment variable prefix for individual models
HELIX_SEED_MODELS_PREFIX="HELIX_SEED_MODEL_"

# Seeding source priority (comma-separated)
HELIX_SEED_MODELS_SOURCES="env,file,defaults"
```

## Integration Points

### Server Startup
```go
// In server initialization
func (s *HelixAPIServer) seedModelsFromEnvironment(ctx context.Context) error {
    if !s.shouldSeedModels() {
        return nil
    }
    
    seedModels, err := s.loadSeedModels()
    if err != nil {
        return fmt.Errorf("failed to load seed models: %w", err)
    }
    
    return s.Store.SeedModels(ctx, seedModels)
}
```

### Store Interface
```go
// Add to store interface
SeedModels(ctx context.Context, models []*types.Model) error
```

## Example Use Cases

### Docker Deployment
```dockerfile
ENV HELIX_SEED_MODEL_1='{"id":"llama3.1:8b","name":"Llama 3.1 8B","type":"chat","runtime":"ollama","memory":"8GB"}'
ENV HELIX_SEED_MODEL_2='{"id":"stable-diffusion-xl","name":"SDXL","type":"image","runtime":"diffusers","memory":"12GiB"}'
```

### Kubernetes ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: helix-models
data:
  models.json: |
    [
      {
        "id": "llama3.1:8b",
        "name": "Llama 3.1 8B",
        "type": "chat",
        "runtime": "ollama",
        "memory": "8GB"
      }
    ]
---
# Mount as HELIX_SEED_MODELS_FILE=/etc/models/models.json
```

### Development Environment
```bash
# .env file
HELIX_SEED_MODELS_ENABLED=true
HELIX_SEED_MODEL_DEV='{"id":"llama3.1:8b","name":"Dev Model","type":"chat","runtime":"ollama","memory":"4GB","enabled":true}'
```

## Migration and Compatibility

### Existing Model Handling
- **System models**: `user_modified=false` - can be updated by seeding
- **User models**: `user_modified=true` - never touched by seeding
- **New models**: Added with `user_modified=false`

### Memory Format Support
The `memory` field supports both numeric (bytes) and human-readable string formats:

**Supported Units:**
- **Binary units (1024-based)**: `B`, `KiB`, `MiB`, `GiB`, `TiB` 
- **Decimal units (1000-based)**: `KB`, `MB`, `GB`, `TB`
- **Numeric**: Raw bytes as integer

**Examples:**
```json
{"memory": "8GB"}        // 8 billion bytes
{"memory": "8GiB"}       // 8 gibibytes (8 * 1024^3 bytes)  
{"memory": 8589934592}   // Raw bytes (8GB)
{"memory": "16.5GB"}     // Decimal values supported
```

### Validation
- JSON schema validation for all seed models
- Memory format parsing with unit conversion
- Runtime argument validation (supports both flattened arrays and nested objects)
- Duplicate ID detection and handling

This approach provides maximum flexibility while keeping the environment variable interface clean and manageable.
