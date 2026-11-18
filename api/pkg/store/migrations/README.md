# How to use migrations

Migrations are used to edit existing tables and help with existing Helix installations. Use sparingly, avoid if possible.

## Schema changes must be expand-only

app.helix.ml deploys blue/green: `helix serve --migrate-only` migrates the
database while the **previous release keeps serving**, then the new release
starts next to it. For up to a few minutes both releases run against the new
schema, and a rollback runs the previous release against it indefinitely. So
every migration (GORM AutoMigrate, the SQL files here, and the hand-written
steps in `runMigrations`) must leave a schema the previous release can still
use:

- **Allowed:** new tables, new nullable columns, new columns with a default,
  new indexes (build them with `createIndexConcurrently`), widening a
  `varchar`.
- **Not allowed in one release:** dropping or renaming a table or column,
  changing a column's type, making a column `NOT NULL`, adding a `NOT NULL`
  column without a default to an existing table.

To remove or rename something, do it over two releases: release N stops
reading and writing it (and adds the replacement), release N+1 drops it.

CI enforces this (`schema-compat` in `.drone.yml`, `api/cmd/schema-compat`): it
migrates a scratch database with the latest release, then with your commit,
and fails on any of the changes above. When the previous release provably no
longer uses a column (the second half of a two-release removal), add
`table.column` to `api/pkg/store/schemacompat/allow.txt` with a comment.

## Migrations must be cheap and safe to repeat

- Every migration connection runs with `lock_timeout=5s`. A step that can't
  get its lock retries a few times and then the deploy fails, instead of
  queueing an `ACCESS EXCLUSIVE` request that blocks every reader behind it.
- A migration that has already been applied must issue no DDL and no writes
  (`TestRunMigrationsIsIdempotent`). Guard hand-written statements with a
  catalog check (`columnDefault`, `columnNullable`, `exists`).
- Backfills go through `backfillRows`: at most 1000 rows per statement, each
  its own transaction, and the condition must stop matching once a row is
  updated.

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

For example renaming the table from `session` to `sessions` (over two
releases, see above: this one-step rename would break the previous release):

```sql
ALTER TABLE session RENAME TO sessions;
```
