# From Helix < 0.6 to Helix 0.6

This version has a significant upgrade of keycloak from Keycloak 15.0.2 to Keycloak 23.0.6. There have been a number of security vulnerabilities addressed with Keycloak, feature support and admin UI changes between these versions.

This version also migrates from using Keycloak's own embedded Java H2 relational database to using Postgres as the backend data store for Keycloak.

This guide provides instructions for migrating a running version of Helix <= 0.5 to Helix 0.6.

## Local development upgrades

This will DESTROY your entire local database (e.g. sessions) as well as your users database.

```
./stack stop
```

Remove postgres volumes with
```
docker volume rm helix_helix-postgres-db
```
This will force the creation of the postgres db needed for keycloak on the next step.

```
./stack up
```

## Production environment upgrades

1. Backup all your data on Keycloak 15.0.2

```
docker exec -it helix-keycloak-1 /opt/jboss/keycloak/bin/standalone.sh -Djboss.socket.binding.port-offset=100 -Dkeycloak.migration.action=export -Dkeycloak.migration.provider=singleFile -Dkeycloak.migration.realmName=helix -Dkeycloak.migration.usersExportStrategy=REALM_FILE -Dkeycloak.migration.file=/tmp/helix_realm.json
```

Press ctrl+c after you see it print:
```
11:30:51,563 INFO  [org.keycloak.exportimport.singlefile.SingleFileExportProvider] (ServerService Thread Pool -- 54) Exporting realm 'helix' into file /tmp/helix_realm.json
11:30:52,544 INFO  [org.keycloak.services] (ServerService Thread Pool -- 54) KC-SERVICES0035: Export finished successfully
```

2. Then copy the backup file out of the container
```
docker cp helix-keycloak-1:/tmp/helix_realm.json helix_realm.json
```

3. Create a Postgres database for Keycloak
```
docker exec -it helix-postgres-1 psql -U postgres
```

on the psql interface
```
create database keycloak;
```

Note: This is only required if you have running volumes of postgres. For any new installations of Helix, this is provided by the docker-compose file and `scripts/postgres/postgres-db.sh`

4. Upgrade/restart the helix stack with

```
git pull; git checkout 0.6.1; docker-compose pull; docker-compose up -d
```

5. Import the keycloak realm and user metadata back into the container

Copy the imported Keycloak backups into the container
```
docker cp helix_realm.json helix-keycloak-1:/tmp/helix_realm.json
```

Import the Keycloak realm and user metadata with
```
docker exec -it helix-keycloak-1 /opt/keycloak/bin/kc.sh import --file /tmp/helix_realm.json
```

6. Restart keycloak after the import completes with
```
docker-compose restart keycloak
```

## Troubleshooting your upgrade

Watch closely, if you get an error about "script uploads are disabled", follow [this guide](https://medium.com/@ramanamuttana/script-upload-is-disabled-in-keycloak-4cb22d9358c8) on how to delete the `authorizationSettings` node from the JSON file.

Remember to run `docker cp` again after you do this!

Once the import of user and realm configuration metadata is complete, Keycloak v.23 should be up and running.


If Keycloak v.23 does not start up, look at `docker logs helix-keycloak-1`

Check Postgres database 'keycloak' exists in
```
docker exec -it helix-postgres-1 psql -U {POSTGRES_USER}
```
```
psql> \list
```

If the realm 'Helix' does not show up on the keycloak admin UI, you can force this to re-appear by creating another new realm (and deleting it later), as this seems to refresh the cache of known realms shown in the UI.

If you encounter issues, please reach out to the Helix team on [Discord](https://discord.com/channels/1180827321704390657/1209590511745106022)
