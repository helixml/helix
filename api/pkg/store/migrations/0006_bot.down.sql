drop table bot;
ALTER TABLE session
DROP COLUMN parent_bot;
ALTER TABLE session
DROP COLUMN child_bot;