-- Set initial prewarm values for default models
-- This is a one-time migration to enable prewarming for commonly used models

-- Enable prewarming for Ollama models
UPDATE models SET prewarm = true WHERE name = 'llama3.1:8b-instruct-q8_0' AND owner_type = 'system';

-- Enable prewarming for VLLM models  
UPDATE models SET prewarm = true WHERE name = 'Qwen/Qwen2.5-VL-3B-Instruct' AND owner_type = 'system';
UPDATE models SET prewarm = true WHERE name = 'Qwen/Qwen2.5-VL-7B-Instruct' AND owner_type = 'system';
UPDATE models SET prewarm = true WHERE name = 'MrLight/dse-qwen2-2b-mrl-v1' AND owner_type = 'system';

-- Ensure diffusers models remain disabled by default (they use too much memory)
UPDATE models SET prewarm = false WHERE model_type = 'diffusers' AND owner_type = 'system'; 