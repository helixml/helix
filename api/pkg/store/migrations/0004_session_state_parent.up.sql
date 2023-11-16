ALTER TABLE session
ADD COLUMN state VARCHAR(255) NOT NULL DEFAULT 'complete',
ADD COLUMN parent_session varchar(255) NOT NULL DEFAULT '';