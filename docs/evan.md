```bash
ssh luke@node05.lukemarsden.net
cd ~/pm/helix
export WITH_RUNNER=1
export KEYCLOAK_FRONTEND_URL=http://node05.lukemarsden.net/auth/
export SERVER_URL=http://node05.lukemarsden.net
./stack start
```

This will run the stack with the runner enabled.

To enable tmux mode it's `ctrl+a` (NOTE: this is normally `ctrl+b` on machines that are not Luke's)
To enable mouse clicks: `ctrl+a` then `:set mouse`
Now you can click around panes.

Now you can open http://node05.lukemarsden.net in your browser.

To stop the stack `ctrl+a` the `d` - this will exit the tmux session.

```bash
./stack stop
```

If there are complaints about node modules in the top window - you need to rebuild:

```bash
./stack stop
docker-compose build
./stack start
```