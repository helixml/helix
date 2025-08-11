# Testing Hugging Face Token Implementation

## Overview

This guide shows how to test the new centralized Hugging Face token management system.

## Prerequisites

1. **Admin Access**: You need admin privileges to manage system settings
2. **Running System**: Control plane and at least one runner connected
3. **HF Token**: A valid Hugging Face token for testing

## Step 1: Set Global HF Token

### Via CLI (Recommended)
```bash
# Get current system settings
helix system settings get

# Set HF token
helix system settings set --huggingface-token "hf_your_token_here"

# Clear HF token (falls back to environment variable)
helix system settings set --clear-hf-token
```

### Via API (Alternative)
```bash
# Get current system settings (admin required)
curl -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  http://localhost:8080/api/v1/system/settings

# Set HF token (admin required)
curl -X PUT \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"huggingface_token": "hf_your_token_here"}' \
  http://localhost:8080/api/v1/system/settings

# Response shows token is set but value is masked:
# {"id":"system","created":"...","updated":"...","huggingface_token_set":true}
```

## Step 2: Create VLLM Model with HF Token Dependency

Use the comprehensive VLLM model example from the CLI help:

```bash
# Create a model that requires HF token (like Qwen 2.5 VL)
helix model create Qwen/Qwen2.5-VL-7B-Instruct \
  --name "Qwen 2.5 VL 7B" \
  --type chat \
  --runtime vllm \
  --memory 39GB \
  --context 32768 \
  --runtime-args '["--trust-remote-code", "--max-model-len", "32768"]'
```

Or create from JSON file:
```bash
# Create model.json with the comprehensive example from CLI help
cat > model.json << EOF
{
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
    "--limit-mm-per-prompt", "{\"image\":10}",
    "--enforce-eager",
    "--max-num-seqs", "64"
  ]
}
EOF

helix model create --file model.json
```

## Step 3: Verify Token Flow

### Check Control Plane Logs
Look for these log messages:
```
INFO system settings updated by admin user_id=admin_user_id hf_token_updated=true
INFO initiated system settings sync to all runners
```

### Check Runner Logs
Look for these log messages:
```
INFO updated hugging face token from control plane runner_id=runner_id token_provided=true
INFO successfully synced system settings to runner runner_id=runner_id hf_token_sent=true
```

### Check Model Startup Logs
When creating a slot with the model, look for:
```
DEBUG NewVLLMRuntime received args model=Qwen/Qwen2.5-VL-7B-Instruct hf_token_provided=true
```

## Step 4: Test Token Resolution Hierarchy

### Test 1: Control Plane Token (Current)
1. Set HF token via API (Step 1)
2. Create VLLM model (Step 2) 
3. Model should use control plane token

### Test 2: Environment Variable Fallback
1. Clear HF token: `curl -X PUT ... -d '{"huggingface_token": ""}'`
2. Set `HF_TOKEN=hf_env_token` on runner
3. Restart runner
4. Create VLLM model
5. Model should use environment token

### Test 3: No Token (Should still work for public models)
1. Clear both control plane and environment tokens
2. Create model with public Hugging Face model
3. Should work without authentication

## Step 5: Test Live Token Updates

1. Create and start a VLLM model
2. Update HF token via API
3. Check runner logs for sync message
4. Create another VLLM model
5. New model should use updated token

## Expected Behavior

### ✅ Success Indicators
- API returns `"huggingface_token_set": true` when token is set
- Runner logs show successful token sync
- VLLM models start successfully with private repos
- New runners automatically receive current token
- Token updates propagate to all runners

### ❌ Failure Indicators  
- API returns 403 Forbidden (need admin privileges)
- Runner logs show token sync failures
- VLLM models fail to start with authentication errors
- Models fall back to environment variables unexpectedly

## Security Notes

- **Tokens are never logged** - only boolean `token_provided`/`hf_token_sent` flags
- **Admin privileges required** for all system settings operations
- **In-memory storage** on runners (not persisted to disk)
- **Secure fallback** to environment variables for backward compatibility

## Future Testing (Per-Org/Per-User Tokens)

When per-org/per-user tokens are implemented, test the resolution hierarchy:

1. **Model-specific token** (highest priority)
2. **User-specific token**
3. **Organization-specific token** 
4. **Global system token** (current implementation)
5. **Environment variable** (lowest priority)

Each level should override the lower levels as expected.
