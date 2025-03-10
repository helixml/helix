# How to use migrations

Migrations are used to edit existing tables and help with existing Helix installations. Use sparingly, avoid if possible.

## DO NOT USE THIS

Do not create tables or add columns through migrations, we use gorm to do this.

```sql
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
```

## DO THIS

For example renaming the table from `session` to `sessions`:

```sql
ALTER TABLE session RENAME TO sessions;
```
