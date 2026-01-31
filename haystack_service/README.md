# Helix Haystack Service

## Development

### Environment

This project is managed by [uv](https://docs.astral.sh/uv/). To get started:

```sh
cd haystack-service
make install
```

### Developing

This will load uvicorn in hot-reload mode during development.

```sh
make dev
```

### Testing

This project uses pytest for testing.

```sh
make test
```

### Linting

This project uses ruff for linting. I recommend you install
[ruff](https://marketplace.visualstudio.com/items?itemName=charliermarsh.ruff) in vscode for
automatic linting and fixing.

```sh
make lint
```

### Building

The production container is built by drone. But you can test the docker build with:

```sh
make build
```

### Running

Run the Docker container with:

```sh
make docker
```
