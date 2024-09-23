# Testing

## Prerequisites

- Install k6 (<https://k6.io/docs/get-started/installation/>).
- Get your Helix API key

## Testing Apps

First, set your Helix API key as an environment variable, e.g.:

```
export HELIX_API_KEY=hl-xxxx
```

By default you can load this <https://github.com/helixml/run-python-helix-app> app to test.

```
k6 run --vus 10 --duration 30s scripts/k6/app.js
```

## Hammering the OpenAI endpoint

First, set your Helix API key as an environment variable, e.g.:

```
export HELIX_API_KEY=hl-xxxx
```

Then hit the API. Change the number of concurrent requests (`--vus`) to match whatever you're trying
to test.

```
k6 run --vus 2 --duration 300s scripts/k6/openai.js
```
