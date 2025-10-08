-- Create zed_settings_overrides table for storing user-specific Zed settings
CREATE TABLE IF NOT EXISTS zed_settings_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create index on user_id for fast lookups
CREATE INDEX IF NOT EXISTS idx_zed_settings_overrides_user_id ON zed_settings_overrides(user_id);

-- Create unique constraint to ensure one settings record per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_zed_settings_overrides_user_id_unique ON zed_settings_overrides(user_id);
