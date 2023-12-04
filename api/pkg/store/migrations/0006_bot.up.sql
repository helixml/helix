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

  -- the session that this bot was created from
  parent_session varchar(255) NOT NULL,

  -- these values are filled in by the parent_session
  lora_dir varchar(255) NOT NULL,
  type varchar(255) NOT NULL,
  model_name varchar(255) NOT NULL
  
);

-- if a session is created from a bot
-- (i.e. a user says "talk to bob")
-- then let's record the bot that spawned the new session
ALTER TABLE session
ADD COLUMN parent_bot varchar(255) NOT NULL DEFAULT '';