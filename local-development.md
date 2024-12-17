# HelixML Local Development guide

## Table of Contents

- [HelixML Local Development guide](#helixml-local-development-guide)
  - [Table of Contents](#table-of-contents)
  - [Introduction](#introduction)
  - [Prerequisites](#prerequisites)
  - [Setting Up the Development Environment](#setting-up-the-development-environment)
  - [Project Structure](#project-structure)
  - [Running the Application](#running-the-application)
    - [1. Bring up the Helix stack](#1-bring-up-the-helix-stack)
    - [2. Attach a runner](#2-attach-a-runner)
      - [Connecting to a Runner via SSH](#connecting-to-a-runner-via-ssh)
      - [Connecting to a Runner via WebhookRelay](#connecting-to-a-runner-via-webhookrelay)
    - [3. (Optional) Expose a Github Webhook](#3-optional-expose-a-github-webhook)
    - [4. Rebuild individual components](#4-rebuild-individual-components)
    - [5. Running tests](#5-running-tests)
    - [6. Tear down the Helix stack](#6-tear-down-the-helix-stack)
  - [Debugging](#debugging)
  - [Contributing](#contributing)
  - [Further Reading](#further-reading)

## Introduction

Welcome to the local development documentation for **Helix.ml**! This guide will help you set up your development environment, understand the project structure, run the application locally, and contribute to the Helix.

## Prerequisites

Before you start, ensure you have the following software installed on your machine:

- **docker**
- **golang**
- **Node.js** and **npm**

## Setting Up the Development Environment

1. **Clone the Repository**

   ```bash
   git clone git@github.com:helixml/helix.git
   cd helix
   ```

    If you are an external contributor, consider working out of a forked repository of Helix.

2. **Set Up Environment Variables**

    Create an `.env` file with settings based on the example values and edit it:

    ```
    cp .env.example-prod .env
    ```

    The default values for settings are optimised for local development.


## Project Structure

Here is an overview of the project structure:

```
helix/
├── Dockerfiles         # Dockerfiles for various environments
├── api/                # Main Control Plane API directory
│   ├── cmd/            # Standard golang project structure within here
│   ├── pkg/            #
│   ├── main.go         #
├── llamaindex/         # llamaindex
│   └── src/            #
│   └── ...             # Other app-specific files
├── unstructured        # Python Unstructured for parsing content
├── scripts             # Scripts to get stuff done
├── runner              # Runner configurations
├── frontend/           # Frontend in React, ts
│   ├── package.json    # npm dependencies
│   └── src/            # Source files for the frontend
└── .env                # Environment variables file
```

## Running the Application

### 1. Bring up the Helix stack

```bash
./stack up
```
This will bring up the control plane which serves the front-end and various other components on the stack. Refer Helix architecture [docs](https://docs.helix.ml/helix/getting-started/architecture/)

The control comes up on http://localhost:8080 by default.

Sanity check your environment with

```
docker ps
```

This should show you the running containers

```
$ docker ps
IMAGE                                       PORTS                                       NAMES
ankane/pgvector                             0.0.0.0:5433->5432/tcp, :::5433->5432/tcp   helix-pgvector-1
helix-frontend                              0.0.0.0:8081->8081/tcp, :::8081->8081/tcp   helix-frontend-1
helix-gptscript_runner                                                                  helix-gptscript_runner-1
registry.helix.ml/helix/llamaindex:latest                                               helix-llamaindex-1
webhookrelay/webhookrelayd                                                              helix-webhook_relay_github-1
webhookrelay/webhookrelayd                                                              helix-webhook_relay_stripe-1
helix-api                                   0.0.0.0:8080->80/tcp, :::8080->80/tcp       helix-api-1
postgres:12.13-alpine                       0.0.0.0:5432->5432/tcp, :::5432->5432/tcp   helix-postgres-1
quay.io/keycloak/keycloak:23.0              8080/tcp, 8443/tcp                          helix-keycloak-1
```

### 2. Attach a runner

Follow the [instructions on the docs to attach a runner](https://docs.helix.ml/helix/private-deployment/controlplane/#attaching-a-runner)

If you're local machine isn't able to host a runner, you have a few options:

- use a VSCode remote SSH session to develop within a machine that does have the resources.
- spin up a remote runner in Luke's bunker (or equivalent) and connect it back to your localhost via an SSH tunnel.
- spin up a remote runner in runpod (or equivalent) and use webhookrelay to connect that machine back to your localhost.

#### Connecting to a Runner via SSH

To connect your localhost to a remote runner via an SSH tunnel, follow these steps:

1. In a separate window, SSH into a remote machine and open a connection from the remote back to local:

    ```bash
    ssh -p $SSH_PORT -R 8080:localhost:8080 user@remote.com
    ```

    Where 8080 is the port that your local API is running on.

2. On the remote: `git clone https://github.com/helixml/helix` somewhere

3. On the remote create a `.env` file with the following settings:

    ```dotenv
    SERVER_PORT=9080 # By default, the runner runs on 8080, so use another port.
    API_HOST=http://localhost:8080 # You've just forwarded this port back to your local machine
    API_TOKEN=oh-hallo-insecure-token # This should match the control plane
    ```

4. On the remote start the runner: `docker compose -f docker-compose.runner.yaml up -d`

5. Now go back to your local machine and browse to `/dashboard` in Helix. You should see the runner. If not, take a look at the runner logs on the remote.

#### Connecting to a Runner via WebhookRelay

To connect to a runner via WebhookRelay:

1. Create a free account with https://webhookrelay.com/
2. Create a new access token and make a note of it: https://my.webhookrelay.com/tokens
3. Create a new "tunnel": https://my.webhookrelay.com/tunnels. Give it a name of `helix`, based in the `eu`, with a destination of `http://localhost:8080`. Click create. Make a note of the "Host" that looks something like: `
https://9hxxxxxxxxx.webrelay.io`
4. Add or edit your `docker-compose.dev.yaml` to include the following configuration. Here we're using host network mode so it can connect to the api which is exposed on 8080. You could change the destination in the previous step to point to somewhere else (like `api:80` for example).

    ```yaml
    webhook_relay_localhost:
        image: webhookrelay/webhookrelayd
        network_mode: "host"
        environment:
        - KEY=${WEBHOOK_RELAY_KEY:-}
        - SECRET=${WEBHOOK_RELAY_SECRET:-}
        command: ["--mode", "tunnel", "-t", "helix"]
    ```

5. Start your remote runner with the API_HOST variable set to the webrelay.io url, e.g.: `--api-host https://xxxx.webrelay.io`.
6. Watch the runner logs to make sure it's able to connect to your local helix.

### 3. (Optional) Expose a Github Webhook

If you're testing, developing or working with Apps then you will need to connect a Github OAuth app to be able to read from user's repositories. The current way of doing this is via https://webhookrelay.com/.

1. Create a free account with https://webhookrelay.com/
2. Create a "bucket", with "forward to an internal location: https://my.webhookrelay.com/new-internal-destination
3. Set the `Destination URL` to: `http://localhost:8080/api/v1/github/webhook`
4. This should produce a url that looks something like `https://xxxx.hooks.webhookrelay.com`
5. Go to the Github oauth app settings and find the HelixML app [called Helix apps development](https://github.com/organizations/helixml/settings/applications/2543199). Generate a new secret and grab the client ID.
6. Set the following environmental variables in the `.env` file:

```dotenv
GITHUB_INTEGRATION_ENABLED=true
GITHUB_INTEGRATION_CLIENT_ID=6d04xxxxxxxx
GITHUB_INTEGRATION_CLIENT_SECRET=xxxxxx
GITHUB_INTEGRATION_WEBHOOK_URL=https://3nmlcxxxxx.hooks.webhookrelay.com/api/v1/github/webhook
```

7. Restart the API and create an app.
8. If you have any problems check the logs in webhook relay bucket.


### 4. Rebuild individual components

```
./stack up --build <component>
```

If you're familiar with [tmux](https://github.com/tmux/tmux/wiki) you will find it useful to do `./stack start` and `./stack stop` instead.

Both the frontend and the api have hot-reloads when in development mode. Rebuilds should only be required when adding libraries.

### 5. Running linter

> [!IMPORTANT]
> You must install [golangci-lint](https://golangci-lint.run/)
> See [here](https://golangci-lint.run/welcome/install/) for installation options.

```
./stack lint
```

### 6. Running tests

```
./stack test
```

### 7. Tear down the Helix stack

Bring down the stack

```
./stack stop
```


## Debugging

- **View all Docker logs**

    ```
    docker logs <container-name>
    ```
## Contributing

1. **Branching Strategy**

   - Create a new branch for each feature or bugfix:

     ```bash
     git checkout -b feature/your-feature-name
     ```

2. **Code Style**

   - Format all code with standard language formatters.
   - Follow the project's coding guidelines for the frontend.
   - Run [Go linter](https://golangci-lint.run/)

3. **Commit Messages**

   - Write clear and concise commit messages.

4. **Pull Requests**

   - Submit a pull request to the `main` branch for review.


## Further Reading

- [Helix Documentation](https://docs.helix.ml/)

Happy coding! If you have any questions or run into issues, feel free to reach out to the maintainers on [👥 Discord](https://discord.gg/VJftd844GE).
