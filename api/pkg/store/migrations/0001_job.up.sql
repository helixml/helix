create table job (
  id varchar(255) PRIMARY KEY,
  created timestamp default current_timestamp,
  owner varchar(255) NOT NULL,
  owner_type varchar(255) NOT NULL,
  state varchar(255) NOT NULL,
  status text,
  -- this is the JSON representation of the job data
  data json not null
);