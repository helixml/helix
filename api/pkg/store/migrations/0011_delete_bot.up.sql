drop table bot;
ALTER TABLE session
RENAME COLUMN parent_bot TO parent_app;
ALTER TABLE session
RENAME COLUMN child_bot TO child_app;
