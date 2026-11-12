# Upgrading PostgreSQL on a Docker Compose install

The self-host `docker-compose.yaml` runs `postgres:12.22-alpine` by default.
PostgreSQL 12 is end-of-life and no longer receives security fixes, so move
to a supported major version. The default does not change by itself: a newer
PostgreSQL cannot start on a data directory written by 12, so the upgrade is a
dump and restore that you run once.

Kubernetes installs are not affected: the Helm chart already runs PostgreSQL 17
(see `charts/helix-controlplane/UPGRADE.md`).

## How it works

Two `.env` settings select the database:

- `HELIX_POSTGRES_IMAGE`: the PostgreSQL image (default `postgres:12.22-alpine`).
- `POSTGRES_DATA`: where the data lives (default: the `helix-postgres-db`
  Docker volume).

The upgrade dumps the old database, points both settings at a new image and an
empty data directory, and restores the dump there. The old volume is never
modified, so rolling back is removing the two lines again.

## Steps

Run these from the install directory (default `/opt/HelixML`). Use `sudo`
where your install needs it. If you run embedded TLS
(`HELIX_VHOST_TLS_MODE=auto`), add `-f docker-compose.yaml -f docker-compose.tls.yaml`
to every `docker compose up`. Expect downtime for the length of the dump and
restore.

1. Stop everything that writes to the database, leaving PostgreSQL running:

   ```bash
   docker compose stop api
   ```

2. Dump all databases. `--no-role-passwords` keeps the old password hashes out
   of the dump; PostgreSQL 14+ authenticates with SCRAM, so the new server
   keeps the password it is initialised with from `POSTGRES_ADMIN_PASSWORD`.

   ```bash
   docker compose exec -T postgres pg_dumpall -U postgres --no-role-passwords > postgres12-backup.sql
   ```

   Check the file is not empty and ends with `PostgreSQL database cluster dump complete`.

3. Stop PostgreSQL and select the new version and an empty data directory:

   ```bash
   docker compose stop postgres
   sudo mkdir -p /opt/HelixML/data/helix-postgres-17
   cat >> .env <<'EOF'
   HELIX_POSTGRES_IMAGE=postgres:17-alpine
   POSTGRES_DATA=/opt/HelixML/data/helix-postgres-17
   EOF
   ```

   If `POSTGRES_DATA` is already set in `.env`, edit that line instead of
   adding a second one.

4. Start the new server and restore the dump into it:

   ```bash
   docker compose up -d postgres
   # wait until it accepts connections
   until docker compose exec -T postgres pg_isready -U postgres; do sleep 2; done
   docker compose exec -T postgres psql -U postgres -q -f - < postgres12-backup.sql
   ```

   `ERROR: role "postgres" already exists` is expected and harmless: the dump
   starts by creating the `postgres` role, which the new server already
   created at initialisation with your current password. Any other error means
   the restore is incomplete: roll back (below) and investigate before
   retrying.

   Roles other than `postgres` (for example one you created by hand) are
   restored without a password. Set one with
   `docker compose exec postgres psql -U postgres -c "ALTER ROLE <name> PASSWORD '<password>'"`.

5. Start Helix:

   ```bash
   docker compose up -d
   ```

   Sign in and check your apps, projects and sessions are present. Keep
   `postgres12-backup.sql` and the old volume until you are satisfied.

Re-running `install.sh` or `install.sh --upgrade` keeps both settings; the
installer preserves keys it does not manage.

## Rolling back

Stop Helix, delete the `HELIX_POSTGRES_IMAGE` and `POSTGRES_DATA` lines you
added to `.env` (or restore the previous `POSTGRES_DATA` value), and run
`docker compose up -d`. Helix starts on the untouched PostgreSQL 12 volume.
Data written after the upgrade is not carried back.

## Cleaning up

Once the new database is confirmed, remove the dump and the old volume:

```bash
docker volume ls | grep helix-postgres-db
docker volume rm <name from the line above>
```
