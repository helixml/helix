create table bot (
  id varchar(255) PRIMARY KEY,
  -- this is a global namespace
  -- we will limit the number of bots per user
  -- TODO: is this a good idea or do we faff on with username/botname?
  name varchar(255),
  created timestamp default current_timestamp,
  updated timestamp default current_timestamp,
  owner varchar(255) NOT NULL,
  owner_type varchar(255) NOT NULL,
  config json not null
);

ALTER TABLE session
RENAME COLUMN parent_app TO child_bot;
ALTER TABLE session
RENAME COLUMN child_app TO child_bot;
