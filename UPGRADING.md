This version has a significant upgrade of keycloak from Keycloak 15.0.2 to Keycloak 23.0.6. There have been a number of security vulnerabilities addressed with Keycloak, feature support and admin UI changes between these versions.

This version also migrates from using Keycloak's own embedded Java H2 relational database to using Postgres as the backend data store for Keycloak.

This guide provides instructions for migrating a running version of [Helix 0.5.5](https://github.com/helixml/helix/releases/tag/0.5.5) to Helix 0.6.0.

1. Backup all your data on Keycloak 15.0.2

`docker exec -it helix-keycloak-1 /opt/jboss/keycloak/bin/standalone.sh -Djboss.socket.binding.port-offset=100 -Dkeycloak.migration.action=export -Dkeycloak.migration.provider=singleFile -Dkeycloak.migration.realmName=helix -Dkeycloak.migration.usersExportStrategy=REALM_FILE -Dkeycloak.migration.file=/tmp/helix_realm.json`

2. Then copy the backup file out of the container
`docker cp helix-keycloak-1:/tmp/helix_realm.json helix_realm.json`

3. Create a Postgres database for Keycloak
`docker exec -it helix-postgres-1 psql -U {POSTGRES_USER}`

on the psql interface
`create database keycloak;`

Note: This is only required if you have running volumes of postgres. For any new installations of Helix, this is provided by the docker-compose file and `scripts/postgres/postgres-db.sh`

4. Restart the helix stack with `./stack up`

5. Import the keycloak realm and user metadata back into the container

Copy the imported Keycloak backups into the container
`docker cp helix_realm.json helix-keycloak-1:/tmp/helix_realm.json`

Import the Keycloak realm and user metadata with
`docker exec -it helix-keycloak-1 /opt/keycloak/bin/kc.sh import --file /tmp/helix_realm.json`

Once the import of user and realm configuration metadata is complete, Keycloak v.23 should be up and running.

Trouble shooting your upgrade:

If Keycloak v.23 does not start up, look at `docker logs helix-keycloak-1`

Check Postgres database 'keycloak' exists in
`docker exec -it helix-postgres-1 psql -U {POSTGRES_USER}`
`psql> \list`

If you encounter issues, please reach out to the Helix team on [Discord](https://discord.com/channels/1180827321704390657/1209590511745106022)