-- TODO: add created_at
create table api_key (
  owner varchar(255) NOT NULL,
  owner_type varchar(255) NOT NULL,
  key varchar(255) PRIMARY KEY,
  name varchar(255) NOT NULL
);