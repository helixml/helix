ALTER TABLE session
ADD COLUMN config json NOT NULL DEFAULT '{}';
