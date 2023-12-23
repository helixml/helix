## manually fix interactions

If you need to reomve some interactions from a session:

 * ssh to the node
 * `cd /data/helix-app/helix`
 * `./stack psql`
 * `select interactions from session where id = 'XXX';`

Copy it - remove the parts you want - save the JSON to a file.

Now we need to escape so we can write the interactions back:

```
jq -R -s '.' < daf4f77c.json
```

Then you can:

```
update session set interactions = 'XXX' where id = 'XXX';
```