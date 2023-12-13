create table usermeta (
  -- the same ID used by keycloak
  id varchar(255) PRIMARY KEY,
  config json not null
);