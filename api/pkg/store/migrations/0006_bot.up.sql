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

-- if a session is used to create a bot then let's record that in the session
-- the bot itself will also have this session in it's config
-- TODO: normalize this and use cascade delete otherwise we are gonna get into a mess
ALTER TABLE session
ADD COLUMN child_bot varchar(255) NOT NULL DEFAULT '';

-- if a session is created from a bot
-- (i.e. a user says "talk to bob")
-- then let's record the bot that spawned the new session
ALTER TABLE session
ADD COLUMN parent_bot varchar(255) NOT NULL DEFAULT '';

