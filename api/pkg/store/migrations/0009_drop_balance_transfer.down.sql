create table balance_transfer (
  id varchar(255) PRIMARY KEY,
  created timestamp default current_timestamp,
  owner varchar(255) NOT NULL,
  owner_type varchar(255) NOT NULL,
  payment_type varchar(255) NOT NULL,
  amount integer NOT NULL,
  -- this is the JSON representation of the job data
  data json not null
);