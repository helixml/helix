## sync data

How to dump the prod psql db and download it locally to debug issues:

```bash
ssh luke@node01.lukemarsden.net
cd /data/helix-app/helix
docker-compose exec postgres pg_dump --no-owner --clean --username postgres > /tmp/db.sql
exit
scp luke@node01.lukemarsden.net:/tmp/db.sql /tmp/db.sql
cat /tmp/db.sql | ./stack psql_pipe
```