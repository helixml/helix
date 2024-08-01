## speedway local dev

How to run speedway tests against the local development stack.

Each time we do this, we must delete ALL docker volumes (because keycloak :facepalm:)

Make sure the stack is stopped and then run:

```bash
docker volume rm helix_helix-filestore helix_helix-keycloak-db helix_helix-pgvector-db helix_helix-postgres-db
```

Now, we need an nrgok address:

```bash
ngrok http 8080
```

Copy the URL - for example `https://1b82-82-47-46-81.ngrok-free.app`

Then - in a new terminal, we export the variables and start the stack:

```bash
export SERVER_URL=https://1b82-82-47-46-81.ngrok-free.app
export KEYCLOAK_FRONTEND_URL=https://1b82-82-47-46-81.ngrok-free.app/auth/
./stack start
```

Now, in a browser, open the ngrok address (e.g. https://1b82-82-47-46-81.ngrok-free.app) and register a user.

If the ngrok warning page appears - just click the "OK proceed button"

When writing speedway tests - you might need to click the ngrok OK button as the first step - you can use `document.querySelector('button')` as the slector because there is only one button on the page.