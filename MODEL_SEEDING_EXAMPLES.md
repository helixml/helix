# Model Seeding Examples

## Environment Variable Configuration

### Basic Configuration
```bash
# Enable model seeding (default: true)
HELIX_SEED_MODELS_ENABLED=true

# Update existing system models (default: false)
HELIX_SEED_MODELS_UPDATE_EXISTING=false

# Custom prefix for model environment variables (default: HELIX_SEED_MODEL_)
HELIX_SEED_MODELS_PREFIX="HELIX_SEED_MODEL_"
```

## Example Model Definitions

### 1. Basic Ollama Model
```bash
HELIX_SEED_MODEL_1='{
  "id": "llama3.1:8b",
  "name": "Llama 3.1 8B",
  "type": "chat",
  "runtime": "ollama",
  "memory": "8GB",
  "description": "Meta Llama 3.1 8B model via Ollama",
  "enabled": true
}'
```

### 2. VLLM Model with Runtime Args
```bash
HELIX_SEED_MODEL_2='{
  "id": "Qwen/Qwen2.5-VL-7B-Instruct",
  "name": "Qwen 2.5 VL 7B",
  "type": "chat",
  "runtime": "vllm",
  "memory": "39GB",
  "context_length": 32768,
  "description": "Multi-modal vision-language model from Alibaba",
  "enabled": true,
  "auto_pull": true,
  "prewarm": true,
  "runtime_args": [
    "--trust-remote-code",
    "--max-model-len", "32768",
    "--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}",
    "--limit-mm-per-prompt", "{\"image\":10}",
    "--enforce-eager",
    "--max-num-seqs", "64"
  ]
}'
```

### 3. Embedding Model
```bash
HELIX_SEED_MODEL_3='{
  "id": "BAAI/bge-large-en-v1.5",
  "name": "BGE Large EN v1.5",
  "type": "embed",
  "runtime": "vllm",
  "memory": "5GB",
  "context_length": 512,
  "description": "High-quality embedding model for RAG applications",
  "enabled": true,
  "runtime_args": [
    "--task", "embed",
    "--max-model-len", "512",
    "--trust-remote-code"
  ]
}'
```

### 4. Diffusers Image Model
```bash
HELIX_SEED_MODEL_4='{
  "id": "stabilityai/stable-diffusion-xl-base-1.0",
  "name": "Stable Diffusion XL",
  "type": "image",
  "runtime": "diffusers",
  "memory": "12GB",
  "description": "High-quality text-to-image generation model",
  "enabled": true,
  "auto_pull": true
}'
```

## Memory Format Examples

The `memory` field supports multiple formats with both binary (1024-based) and decimal (1000-based) units:

```bash
# Bytes as number
"memory": 8589934592

# Bytes as string
"memory": "8589934592"

# Binary units (powers of 1024) - recommended for memory
"memory": "8GiB"    # 8 * 1024^3 = 8,589,934,592 bytes
"memory": "4096MiB" # 4096 * 1024^2 = 4,294,967,296 bytes
"memory": "1TiB"    # 1 * 1024^4 = 1,099,511,627,776 bytes
"memory": "512KiB"  # 512 * 1024 = 524,288 bytes

# Decimal units (powers of 1000) - standard SI units
"memory": "8GB"     # 8 * 1000^3 = 8,000,000,000 bytes
"memory": "4000MB"  # 4000 * 1000^2 = 4,000,000,000 bytes
"memory": "1TB"     # 1 * 1000^4 = 1,000,000,000,000 bytes
"memory": "512KB"   # 512 * 1000 = 512,000 bytes

# Short forms (binary)
"memory": "8G"      # Same as "8GiB"
"memory": "4096M"   # Same as "4096MiB"
```

**Note:** Use binary units (GiB, MiB) for GPU memory as they match how hardware reports memory. Use decimal units (GB, MB) when you want to match marketing specifications.

## Runtime Args Structure

The `runtime_args` field now supports a **flattened array structure** (recommended) while maintaining backward compatibility with the nested format:

### Recommended Flattened Format
```json
{
  "runtime_args": [
    "--trust-remote-code",
    "--max-model-len", "32768",
    "--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}",
    "--enforce-eager"
  ]
}
```

### Legacy Nested Format (still supported)
```json
{
  "runtime_args": {
    "args": [
      "--trust-remote-code",
      "--max-model-len", "32768"
    ]
  }
}
```

### Template Substitution
Use placeholder substitution for dynamic values instead of hardcoding them:
- `{{.DynamicMemoryUtilizationRatio}}` - Automatically calculated GPU memory utilization
- More placeholders may be available depending on the runtime

**Benefits of flattened format:**
- Cleaner, more intuitive structure
- Direct array of command-line arguments
- Better alignment with how arguments are actually used

## Docker Compose Example

```yaml
version: '3.8'
services:
  helix-api:
    image: helix/api:latest
    environment:
      # Enable model seeding
      - HELIX_SEED_MODELS_ENABLED=true
      - HELIX_SEED_MODELS_UPDATE_EXISTING=false
      
      # Seed models
      - HELIX_SEED_MODEL_OLLAMA={"id":"llama3.1:8b","name":"Llama 3.1 8B","type":"chat","runtime":"ollama","memory":"8GB","enabled":true}
      - HELIX_SEED_MODEL_VLLM={"id":"microsoft/DialoGPT-medium","name":"DialoGPT Medium","type":"chat","runtime":"vllm","memory":"4GB","enabled":true,"runtime_args":["--trust-remote-code"]}
      
      # Global HF token (will be passed to runners)
      - HELIX_HF_TOKEN=hf_your_token_here
```

## Kubernetes ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: helix-model-config
data:
  HELIX_SEED_MODELS_ENABLED: "true"
  HELIX_SEED_MODELS_UPDATE_EXISTING: "false"
  HELIX_SEED_MODEL_1: |
    {
      "id": "llama3.1:8b",
      "name": "Llama 3.1 8B",
      "type": "chat",
      "runtime": "ollama",
      "memory": "8GB",
      "enabled": true
    }
  HELIX_SEED_MODEL_2: |
    {
      "id": "Qwen/Qwen2.5-VL-7B-Instruct",
      "name": "Qwen 2.5 VL 7B",
      "type": "chat",
      "runtime": "vllm",
      "memory": "39GB",
      "context_length": 32768,
      "enabled": true,
      "runtime_args": [
        "--trust-remote-code", 
        "--max-model-len", "32768",
        "--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}"
      ]
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: helix-api
spec:
  template:
    spec:
      containers:
      - name: helix-api
        image: helix/api:latest
        envFrom:
        - configMapRef:
            name: helix-model-config
```

## Testing Model Seeding

### 1. Check Logs
Look for these log messages during startup:
```
INFO loaded seed models from environment count=3
INFO seeding models from environment variables count=3
INFO created model from environment seed model_id=llama3.1:8b name="Llama 3.1 8B"
INFO completed model seeding from environment variables
```

### 2. Verify Models via CLI
```bash
# List all models to see seeded ones
helix model list

# Inspect a specific seeded model
helix model inspect "llama3.1:8b"
```

### 3. Verify Models via API
```bash
# List models
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/helix-models

# Check specific model
curl -H "Authorization: Bearer YOUR_TOKEN" \
  "http://localhost:8080/api/v1/helix-models/llama3.1:8b"
```

## Behavior Details

### Model Updates
- **New models**: Always created from seed data
- **Existing system models** (`user_modified=false`): Updated only if `HELIX_SEED_MODELS_UPDATE_EXISTING=true`
- **User-modified models** (`user_modified=true`): Never updated (preserves user customizations)

### Error Handling
- Invalid JSON in environment variables is logged and skipped
- Missing required fields (id, name, type, runtime) are logged and skipped
- Invalid memory formats are logged and cause model creation to fail
- Startup continues even if some models fail to seed

### Performance
- Seeding runs once during server startup
- Only processes models that need creation/updating
- Uses database transactions for consistency

## Migration from Hardcoded Models

To migrate from hardcoded models in `models.go` to environment-based seeding:

1. **Extract model definitions** from `models.go` into environment variables
2. **Set `HELIX_SEED_MODELS_UPDATE_EXISTING=true`** to update existing models
3. **Test thoroughly** to ensure all models are created correctly
4. **Gradually remove** hardcoded models from `models.go` (optional)

The seeding system is designed to coexist with hardcoded models, so migration can be gradual.
