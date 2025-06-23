-- Revert initial prewarm values for default models
-- This reverts the prewarm settings back to false for all system models

UPDATE models SET prewarm = false WHERE owner_type = 'system'; 